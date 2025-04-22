package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"syscall"

	"os"
	"os/signal"
	"strconv"

	pb "github.com/cc-0000/indeq/common/api"
	"github.com/cc-0000/indeq/common/config"
	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"

	kivik "github.com/go-kivik/kivik/v4"
	_ "github.com/go-kivik/kivik/v4/couchdb"
)

type queryServer struct {
	pb.UnimplementedQueryServiceServer
	rabbitMQConn                   *amqp.Connection
	retrievalConn                  *grpc.ClientConn
	retrievalService               pb.RetrievalServiceClient
	queueTTL                       int
	summaryUpperBound              int
	summaryLowerBound              int
	systemPrompt                   string
	deepInfraApiKey                string
	openAiApiKey                   string
	geminiClient                   *genai.Client
	geminiFlash2ModelHeavy         *genai.GenerativeModel
	geminiFlash2ModelLight         *genai.GenerativeModel
	geminiFlash2ModelSummarization *genai.GenerativeModel
	geminiFlash2ModelTitle         *genai.GenerativeModel
	geminiFlash2ModelYesNoSearch   *genai.GenerativeModel
	couchdbClient                  *kivik.Client
	conversationsDB                *kivik.DB
	ownershipDB                    *kivik.DB
}

// func(client TLS config)
//   - connects to the retrieval service using the provided client tls config and saves the connection and function interface to the server struct
//   - assumes: the connection will be closed in the parent function at some point
func (s *queryServer) connectToRetrievalService(tlsConfig *tls.Config) {
	// Connect to the desktop service
	retrievalAddy, ok := os.LookupEnv("RETRIEVAL_ADDRESS")
	if !ok {
		log.Fatal("failed to retrieve retrieval address for connection")
	}
	retrievalConn, err := grpc.NewClient(
		retrievalAddy,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
	)
	if err != nil {
		log.Fatalf("Failed to establish connection with retrieval-service: %v", err)
	}

	s.retrievalConn = retrievalConn
	s.retrievalService = pb.NewRetrievalServiceClient(retrievalConn)
}

// func()
//   - sets up the gRPC server, connects it with the global struct, and TLS
//   - assumes: you will call grpcServer.GracefulStop() in the parent function at some point
func (s *queryServer) createGRPCServer() *grpc.Server {
	// set up TLS for the gRPC server and serve it
	tlsConfig, err := config.LoadServerTLSFromEnv("QUERY_CRT", "QUERY_KEY")
	if err != nil {
		log.Fatalf("Error loading TLS config for query service: %v", err)
	}

	opts := []grpc.ServerOption{
		grpc.Creds(credentials.NewTLS(tlsConfig)),
	}
	grpcServer := grpc.NewServer(opts...)
	pb.RegisterQueryServiceServer(grpcServer, s)

	return grpcServer
}

// func(pointer to a fully set up grpc server)
//   - starts the query-service grpc server
//   - this is a blocking call
func (s *queryServer) startGRPCServer(grpcServer *grpc.Server) {
	grpcAddress, ok := os.LookupEnv("QUERY_PORT")
	if !ok {
		log.Fatal("failed to find the query service port in env variables")
	}

	listener, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()
	log.Printf("Query gRPC Service listening on %v\n", listener.Addr())

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

// func()
//   - connects to the rabbitMQ broker
//   - assumes: you will call rabbitMQConn.Close() in the parent function at some point
func (s *queryServer) connectToRabbitMQ() {
	// Connect to RabbitMQ
	rabbitMQConn, err := amqp.Dial(os.Getenv("RABBITMQ_URL"))
	if err != nil {
		log.Fatalf("failed to connect to RabbitMQ broker: %v", err)
	}
	s.rabbitMQConn = rabbitMQConn

	queue_ttl, err := strconv.ParseUint(os.Getenv("QUERY_QUEUE_TTL"), 10, 32)
	if err != nil {
		log.Fatal("failed to find the query queue ttl in env variables")
	}
	s.queueTTL = int(queue_ttl)
}

// func(context)
//   - connects to google gemini
//   - assumes: the client will be closed in the parent function at some point
func (s *queryServer) connectToLLMApis() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	geminiApikey, ok := os.LookupEnv("GEMINI_API_KEY")
	if !ok {
		log.Fatalf("failed to retrieve the gemini api key")
	}

	deepInfraApiKey, ok := os.LookupEnv("DEEPINFRA_API_KEY")
	if !ok {
		log.Fatalf("failed to retrieve the deep infra api key")
	}
	s.deepInfraApiKey = deepInfraApiKey

	openAiApiKey, ok := os.LookupEnv("OPENAI_API_KEY")
	if !ok {
		log.Fatalf("failed to retrieve the openai api key")
	}
	s.openAiApiKey = openAiApiKey

	summaryUpperBound, err := strconv.ParseInt(os.Getenv("QUERY_SUMMARY_UPPER_BOUND"), 10, 64)
	if err != nil {
		log.Fatalf("failed to retrieve the query summary upper bound: %v", err)
	}
	s.summaryUpperBound = int(summaryUpperBound)

	summaryLowerBound, err := strconv.ParseInt(os.Getenv("QUERY_SUMMARY_LOWER_BOUND"), 10, 64)
	if err != nil {
		log.Fatalf("failed to retrieve the query summary lower bound: %v", err)
	}
	s.summaryLowerBound = int(summaryLowerBound)

	client, err := genai.NewClient(ctx, option.WithAPIKey(geminiApikey))
	if err != nil {
		log.Fatalf("Error creating client: %v", err)
	}
	s.geminiClient = client

	heavyModel := client.GenerativeModel("gemini-2.0-flash")
	heavyModel.SetTemperature(1)
	heavyModel.SetTopK(1)
	heavyModel.SetTopP(0.95)
	heavyModel.SetMaxOutputTokens(8196)
	heavyModel.ResponseMIMEType = "text/plain"
	systemPrompt := "You are a very helpful assistant called Indeq with knowledge on virtually every single topic. You will ALWAYS find the best answer to the user's query, even if you're missing information from excerpts. Use the conversation history, and any provided excerpts to augment your general knowledge and then answer the question that follows. Always cite excerpts using the <number_of_excerpt_in_question> (for example, when citing Excerpt: 1, use <1>) when using specific information from the excerpts.\n\n"
	heavyModel.SystemInstruction = &genai.Content{
		Parts: []genai.Part{
			genai.Text(systemPrompt),
		},
	}
	s.systemPrompt = systemPrompt
	s.geminiFlash2ModelHeavy = heavyModel

	lightModel := client.GenerativeModel("gemini-2.0-flash")
	lightModel.SetTemperature(1)
	lightModel.SetTopK(1)
	lightModel.SetTopP(0.95)
	lightModel.SetMaxOutputTokens(200)
	systemPrompt = "IMPORTANT: Do NOT answer the query directly. You are to ALWAYS respond using the generate_search_query function.\n" +
		"You are an LLM that is connected to a user's local files, cloud drive(s), email, textbooks, slides, etc. These are referred to as the user's CONNECTIONS.\n" +
		"Based on the user query, generate 3-5 alternative phrasings and key terms as an expanded_query suitable for searching the user's CONNECTIONS.\n" +
		"IMPORTANT: ALWAYS respond by calling the generate_search_query function with the expanded_query field. NEVER respond with plain text."
	lightModel.SystemInstruction = &genai.Content{
		Parts: []genai.Part{
			genai.Text(systemPrompt),
		},
	}
	// Define our function declaration for generating search queries
	generateSearchQueryFunction := &genai.FunctionDeclaration{
		Name: "generate_search_query",
		Description: "Generate 3-5 alternative phrasings and key terms (as expanded_query) based on the user query to search through their CONNECTIONS (local files, cloud drive(s), email, textbooks, slides, etc.).",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,

			Properties: map[string]*genai.Schema{
				"expanded_query": {
					Type:        genai.TypeString,
					Description: "Expanded search terms and phrases based on the user query.",
				},
			},
			Required: []string{"expanded_query"},
		},
	}
	lightModel.Tools = []*genai.Tool{
		{FunctionDeclarations: []*genai.FunctionDeclaration{generateSearchQueryFunction}},
	}
	s.geminiFlash2ModelLight = lightModel

	yesNoSearchModel := client.GenerativeModel("gemini-2.0-flash")
	yesNoSearchModel.SetTemperature(1)
	yesNoSearchModel.SetTopK(1)
	yesNoSearchModel.SetTopP(0.95)
	yesNoSearchModel.SetMaxOutputTokens(80)
	systemPrompt = "IMPORTANT: Do NOT answer the query directly. You are to ALWAYS respond using the should_search_connections function.\n" +
		"You are an LLM that is connected to a user's local files, cloud drive(s), email, textbooks, slides, etc. These are referred to as the user's CONNECTIONS.\n" +
		"INSTRUCTIONS:\n" +
		"1. Based on the user query and history of previous queries, figure out what the user is asking for and talking about.\n" +
		"2. Next, decide whether or not we should search through the user's CONNECTIONS to find the answer to what they are looking for. Answer 'yes' if a search is required, 'no' if the query is basic general knowledge.\n" +
		"IMPORTANT: ALWAYS respond by calling the should_search_connections function with the 'decision' field set to 'yes' or 'no'. NEVER respond with plain text."
	yesNoSearchModel.SystemInstruction = &genai.Content{
		Parts: []genai.Part{
			genai.Text(systemPrompt),
		},
	}
	// Define our function declaration for the search decision
	shouldSearchConnectionsFunction := &genai.FunctionDeclaration{
		Name:        "should_search_connections",
		Description: "Decide whether to search the user's CONNECTIONS (local files, cloud drives, email, etc.) based on the user query.",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,

			Properties: map[string]*genai.Schema{
				"decision": {
					Type:        genai.TypeString,
					Enum:        []string{"yes", "no"},
					Description: "'yes' if searching CONNECTIONS is required, 'no' otherwise.",
				},
			},
			Required: []string{"decision"},
		},
	}
	yesNoSearchModel.Tools = []*genai.Tool{
		{FunctionDeclarations: []*genai.FunctionDeclaration{shouldSearchConnectionsFunction}},
	}
	s.geminiFlash2ModelYesNoSearch = yesNoSearchModel

	summarizationModel := client.GenerativeModel("gemini-2.0-flash-lite")
	summarizationModel.SetTemperature(0.3)
	summarizationModel.SetTopK(1)
	summarizationModel.SetTopP(0.95)
	summarizationModel.SetMaxOutputTokens(1024)
	summarizationModel.ResponseMIMEType = "text/plain"
	systemPrompt = "You are an expert at creating concise summaries of conversations between a human (referred to as the user) and an AI assistant called Indeq.\n\n" +
		"When told to summarize, follow these guidelines:\n" +
		"1. Focus on capturing the key points, questions, and information exchanged.\n" +
		"2. Extract the main topics, questions, and information from the conversation.\n" +
		"3. Identify any decisions made or conclusions reached.\n" +
		"4. Maintain factual accuracy while condensing the exchange.\n" +
		"5. Summarize in third person (e.g., 'The user asked about X, and Indeq explained Y').\n" +
		"6. Be brief but comprehensive, highlighting the most important information.\n" +
		"7. Exclude pleasantries, acknowledgments, and other non-essential dialogue.\n" +
		"8. The summary should be significantly shorter than the original conversation while preserving the essential context needed for understanding the interaction.\n\n"

	summarizationModel.SystemInstruction = &genai.Content{
		Parts: []genai.Part{
			genai.Text(systemPrompt),
		},
	}
	s.geminiFlash2ModelSummarization = summarizationModel

	titleModel := client.GenerativeModel("gemini-2.0-flash-lite")
	titleModel.SetTemperature(0.3)
	titleModel.SetTopK(1)
	titleModel.SetTopP(0.95)
	titleModel.SetMaxOutputTokens(64)
	titleModel.ResponseMIMEType = "text/plain"
	systemPrompt = "You are an expert at creating concise titles for conversations between a human (referred to as the user) and an AI assistant called Indeq.\n\n" +
		"When told to create a title, follow these guidelines:\n" +
		"1. Focus on capturing the key points, questions, user intent, and information exchanged.\n" +
		"2. Maintain factual accuracy while condensing the exchange.\n" +
		"3. The title should be significantly shorter than the original conversation while preserving the essential context needed for understanding the interaction.\n" +
		"4. ONLY output the title and nothing else.\n"

	titleModel.SystemInstruction = &genai.Content{
		Parts: []genai.Part{
			genai.Text(systemPrompt),
		},
	}
	s.geminiFlash2ModelTitle = titleModel
}

// func()
//   - connects to the couchdb database
//   - assumes: you will call couchdbClient.Close() in the parent function at some point
//   - assumes: you will call conversationsDB.Close() in the parent function at some point
func (s *queryServer) connectToCouchDB(ctx context.Context) {
	// retrieve env credentials
	couchdbUser, ok := os.LookupEnv("COUCHDB_USER")
	if !ok {
		log.Fatalf("failed to retrieve the couchdb user")
	}
	couchdbPassword, ok := os.LookupEnv("COUCHDB_PASSWORD")
	if !ok {
		log.Fatalf("failed to retrieve the couchdb password")
	}
	couchdbAddress, ok := os.LookupEnv("COUCHDB_ADDRESS")
	if !ok {
		log.Fatalf("failed to retrieve the couchdb address")
	}
	conversationDBName := "conversations"
	ownershipDBName := "ownership"

	// connect to couchdb
	client, err := kivik.New("couch", fmt.Sprintf("http://%s:%s@%s/", couchdbUser, couchdbPassword, couchdbAddress))
	if err != nil {
		log.Fatalf("failed to connect to couchdb: %v", err)
	}
	s.couchdbClient = client

	// Create or get a database
	exists, err := client.DBExists(ctx, conversationDBName)
	if err != nil {
		log.Fatalf("failed to check if database exists: %v", err)
	} else if !exists {
		// Database doesn't exist, create it
		if err := client.CreateDB(ctx, conversationDBName); err != nil {
			log.Fatalf("failed to create couchdb database: %v", err)
		}
	}

	s.conversationsDB = client.DB(conversationDBName)

	// Create or get a database
	exists, err = client.DBExists(ctx, ownershipDBName)
	if err != nil {
		log.Fatalf("failed to check if database exists: %v", err)
	} else if !exists {
		// Database doesn't exist, create it
		if err := client.CreateDB(ctx, ownershipDBName); err != nil {
			log.Fatalf("failed to create couchdb database: %v", err)
		}
	}

	s.ownershipDB = client.DB(ownershipDBName)
}

func main() {
	// graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load the .env file
	err := config.LoadSharedConfig()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// create the clientTLSConfig for use in connecting to other services
	clientTlsConfig, err := config.LoadClientTLSFromEnv("QUERY_CRT", "QUERY_KEY", "CA_CRT")
	if err != nil {
		log.Fatalf("failed to load client TLS configuration from .env: %v", err)
	}

	// create the server struct
	server := &queryServer{}

	// Connect to RabbitMQ
	server.connectToRabbitMQ()
	defer server.rabbitMQConn.Close()

	// start grpc server
	grpcServer := server.createGRPCServer()
	go server.startGRPCServer(grpcServer)
	defer grpcServer.GracefulStop()

	// Connect to retrieval service
	server.connectToRetrievalService(clientTlsConfig)
	defer server.retrievalConn.Close()

	// Connect to google gemini
	server.connectToLLMApis()
	defer server.geminiClient.Close()

	// Connect to couchdb
	server.connectToCouchDB(ctx)
	defer server.couchdbClient.Close()
	defer server.conversationsDB.Close()

	// listen for shutdown signal
	<-sigChan // TODO: implement worker groups
	log.Print("gracefully shutting down...")
}
