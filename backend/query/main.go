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

	lightModel := client.GenerativeModel("gemini-2.0-flash-lite")
	lightModel.SetTemperature(1)
	lightModel.SetTopK(1)
	lightModel.SetTopP(0.95)
	lightModel.SetMaxOutputTokens(200)
	lightModel.ResponseMIMEType = "text/plain"
	systemPrompt = "IMPORTANT: Do NOT answer the query directly. You are to ALWAYS responds using the handle_query function.\n" +
		"For each user query, you must decide:\n" +
		"- Use \"direct_answer\" when the query is about general knowledge, definitions, or topics that don't require up-to-date information\n" +
		"- Use \"search\" when the query needs recent information, specific data, specialized knowledge, or personal information\n" +
		"IMPORTANT: ALWAYS respond by calling the handle_query function with the appropriate action and expanded_query fields. NEVER respond with plain text."
	lightModel.SystemInstruction = &genai.Content{
		Parts: []genai.Part{
			genai.Text(systemPrompt),
		},
	}
	// Define our function declaration for query handling
	handleQueryFunction := &genai.FunctionDeclaration{
		Name: "handle_query",
		Description: "Process the user query by selecting one of two actions:\n" +
			"1. 'direct_answer' - For general knowledge questions that don't need research\n" +
			"2. 'search' - For queries requiring research or up-to-date information\n\n" +
			"When 'search' is selected, provide 3-5 alternative phrasings and key terms as expanded_query",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,

			Properties: map[string]*genai.Schema{
				"action": {
					Type:        genai.TypeString,
					Enum:        []string{"search", "direct_answer"},
					Description: "Whether to search for more information or provide a direct answer",
				},
				"expanded_query": {
					Type:        genai.TypeString,
					Description: "Expanded search terms and phrases if action is 'search'",
				},
			},
			Required: []string{"action"},
		},
	}
	lightModel.Tools = []*genai.Tool{
		{FunctionDeclarations: []*genai.FunctionDeclaration{handleQueryFunction}},
	}

	s.geminiFlash2ModelLight = lightModel

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
