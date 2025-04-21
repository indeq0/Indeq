package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	pb "github.com/cc-0000/indeq/common/api"
	"github.com/cc-0000/indeq/common/config"
	_ "github.com/lib/pq"
	"github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	kivik "github.com/go-kivik/kivik/v4"
	_ "github.com/go-kivik/kivik/v4/couchdb"
)

type crawlingServer struct {
	pb.UnimplementedCrawlingServiceServer
	integrationConn    *grpc.ClientConn
	integrationService pb.IntegrationServiceClient
	vectorConn         *grpc.ClientConn
	vectorService      pb.VectorServiceClient
	db                 *sql.DB
	kafkaWriter        *kafka.Writer
	CouchdbClient      *kivik.Client
	ChunkIDdb          *kivik.DB
}

type Metadata struct {
	DateCreated      time.Time // Universal timestamp for creation
	DateLastModified time.Time // Universal timestamp for last modification
	UserID           string    // Unique identifier for the user
	ResourceID       string    // Unique ID of the resource (platform-specific)
	ResourceType     string    // Standardized type (e.g., "document", "spreadsheet", "email")
	FileURL          string    // URL
	Title            string    // Title or subject of the resource
	ChunkID          string    // Use to uniquely identify chunks
	FilePath         string    // Folder structure
	Platform         string    // Platform identifier ("GOOGLE", "MICROSOFT", "NOTION")
	Service          string    // Service identifier ("GOOGLE_DRIVE", "GOOGLE_GMAIL")
}

type TextChunkMessage struct {
	Metadata Metadata
	Content  string
}

type File struct {
	File []TextChunkMessage
}

type ListofFiles struct {
	Files []File
}

// Helper function to convert platform string to Provider enum
func convertPlatformToProvider(platform string) pb.Provider {
	switch strings.ToUpper(platform) {
	case "GOOGLE":
		return pb.Provider_GOOGLE
	case "MICROSOFT":
		return pb.Provider_MICROSOFT
	case "NOTION":
		return pb.Provider_NOTION
	default:
		return pb.Provider_PROVIDER_UNSPECIFIED
	}
}

func convertPlatformToEnum(platform string) pb.Platform {
	switch strings.ToUpper(platform) {
	case "GOOGLE":
		return pb.Platform_PLATFORM_GOOGLE
	case "MICROSOFT":
		return pb.Platform_PLATFORM_MICROSOFT
	case "NOTION":
		return pb.Platform_PLATFORM_NOTION
	default:
		return pb.Platform_PLATFORM_LOCAL
	}
}

func (s *crawlingServer) retrieveAccessToken(ctx context.Context, userID string, platform string) (string, error) {
	response, err := s.integrationService.GetAccessToken(ctx, &pb.GetAccessTokenRequest{
		UserId:   userID,
		Platform: convertPlatformToProvider(platform),
	})
	if err != nil {
		return "", fmt.Errorf("error calling GetAccessToken: %v", err)
	}
	if !response.Success {
		return "", fmt.Errorf("failed to retrieve access token: %s", response.Message)
	}
	_, err = ValidateAccessToken(response.AccessToken, platform)
	if err != nil {
		return "", fmt.Errorf("failed to validate access token: %v", err)
	}
	return response.AccessToken, nil
}

func (s *crawlingServer) StartInitalCrawler(ctx context.Context, req *pb.StartInitalCrawlerRequest) (*pb.StartInitalCrawlerResponse, error) {
	scope, err := ValidateAccessToken(req.AccessToken, req.Platform)
	if err != nil {
		return &pb.StartInitalCrawlerResponse{
			Success:      false,
			Message:      fmt.Sprintf("Failed to validate access token: %v", err),
			ErrorDetails: err.Error(),
		}, nil
	}
	err = s.NewCrawler(ctx, req.UserId, req.AccessToken, req.Platform, scope)
	if err != nil {
		return &pb.StartInitalCrawlerResponse{
			Success:      false,
			Message:      fmt.Sprintf("Failed to start initial crawler: %v", err),
			ErrorDetails: err.Error(),
		}, nil
	}

	return &pb.StartInitalCrawlerResponse{
		Success:      true,
		Message:      "Initial crawler started successfully",
		ErrorDetails: "",
	}, nil
}

// Things that will be crawled Google, Microsoft, Notion
func (s *crawlingServer) NewCrawler(ctx context.Context, userID string, accessToken string, platform string, scopes []string) error {
	switch platform {
	case "GOOGLE":
		client := createGoogleOAuthClient(ctx, accessToken)
		err := s.GoogleCrawler(ctx, client, userID, scopes)
		if err != nil {
			log.Printf("Error in GoogleCrawler: %v", err)
		}
		return err
	case "NOTION":
		client := createNotionOAuthClient(ctx, accessToken)
		err := s.NotionCrawler(ctx, client, userID)
		if err != nil {
			log.Printf("Error in NotionCrawler: %v", err)
		}
		return err
	case "MICROSOFT":
		client := createMicrosoftOAuthClient(ctx, accessToken)
		err := s.MicrosoftCrawler(ctx, client, userID)
		if err != nil {
			log.Printf("Error in MicrosoftCrawler: %v", err)
		}
		return err
	default:
		return fmt.Errorf("unsupported platform: %s", platform)
	}
}

// ManualCrawler updates the crawler when user presses update to make sure data is up-to-date
func (s *crawlingServer) ManualCrawler(ctx context.Context, req *pb.ManualCrawlerRequest) (*pb.ManualCrawlerResponse, error) {
	tokens, err := GetRetrievalTokens(ctx, s.db, req.UserId)
	if err != nil {
		return nil, fmt.Errorf("error querying retrieval tokens: %w", err)
	}

	if len(tokens) == 0 {
		return &pb.ManualCrawlerResponse{Success: false}, nil
	}

	for _, token := range tokens {
		accessToken, err := s.retrieveAccessToken(ctx, req.UserId, token.Platform)
		if err != nil {
			log.Printf("Error retrieving access token: %v", err)
			continue
		}

		err = updateCrawlerWithToken(ctx, s, req.UserId, token.Platform, token.Service, token.RetrievalToken, accessToken)
		if err != nil {
			log.Printf("Error updating crawler: %v", err)
			continue
		}
	}

	return &pb.ManualCrawlerResponse{Success: true}, nil
}

// startPeriodicCrawlerWorker starts a periodic crawler worker
func (s *crawlingServer) startPeriodicCrawlerWorker(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 30)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				log.Println("Running periodic crawler worker...")
				s.UpdateDBCrawler()
			}
		}
	}()
}

// UpdateDBCrawler updates the crawler with new access tokens to make sure data is up-to-date
func (s *crawlingServer) UpdateDBCrawler() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	tokens, err := GetOutdatedTokens(ctx, s.db)
	if err != nil {
		log.Printf("Error querying outdated tokens: %v", err)
		return
	}
	for _, token := range tokens {
		accessToken, err := s.retrieveAccessToken(ctx, token.UserID, token.Platform)
		if err != nil {
			log.Printf("Error retrieving access token: %v", err)
			continue
		}
		err = updateCrawlerWithToken(ctx, s, token.UserID, token.Platform, token.Service, token.RetrievalToken, accessToken)
		if err != nil {
			log.Printf("Error updating crawler: %v", err)
			continue
		}
	}
}

// updateCrawlerWithToken updates the crawler with a new access token
func updateCrawlerWithToken(ctx context.Context, s *crawlingServer, userID, platform, service, retrievalToken, accessToken string) error {
	if platform == "GOOGLE" {
		newRetrievalToken, err := s.UpdateCrawlGoogle(ctx, createGoogleOAuthClient(ctx, accessToken), service, userID, retrievalToken)
		if err != nil {
			log.Printf("Error updating crawler: %v", err)
			return err
		}
		if err := storeRetrievalToken(ctx, s.db, userID, platform, service, newRetrievalToken); err != nil {
			return err
		}
		return nil
	} else if platform == "NOTION" {
		newRetrievalToken, err := s.UpdateCrawlNotion(ctx, createNotionOAuthClient(ctx, accessToken), userID, retrievalToken)
		if err != nil {
			log.Printf("Error updating crawler: %v", err)
			return err
		}
		if err := storeRetrievalToken(ctx, s.db, userID, platform, service, newRetrievalToken); err != nil {
			return err
		}
		return nil
	} else if platform == "MICROSOFT" {
		newRetrievalToken, err := s.UpdateCrawlMicrosoft(ctx, createMicrosoftOAuthClient(ctx, accessToken), userID, retrievalToken)
		if err != nil {
			log.Printf("Error updating crawler: %v", err)
			return err
		}
		if err := storeRetrievalToken(ctx, s.db, userID, platform, service, newRetrievalToken); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("unsupported platform: %s", platform)
}

func (s *crawlingServer) DeleteCrawlerData(ctx context.Context, req *pb.DeleteCrawlerDataRequest) (*pb.DeleteCrawlerDataResponse, error) {
	_, err := DeleteRetrievalTokens(ctx, s.db, req.UserId, req.Platform)
	if err != nil {
		return &pb.DeleteCrawlerDataResponse{
			Success: false,
			Message: fmt.Sprintf("Database error deleting retrieval tokens: %v", err),
		}, nil
	}
	_, err = s.db.ExecContext(ctx, deleteProcessingStatusQuery, req.UserId, req.Platform)
	if err != nil {
		return &pb.DeleteCrawlerDataResponse{
			Success: false,
			Message: fmt.Sprintf("Database error deleting processing status: %v", err),
		}, nil
	}

	if err := s.DeleteChunkMappingsForPlatform(ctx, req.UserId, req.Platform); err != nil {
		return &pb.DeleteCrawlerDataResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to delete chunk mappings: %v", err),
		}, nil
	}

	if err := s.deleteFilesFromVector(ctx, req.UserId, req.Platform); err != nil {
		return &pb.DeleteCrawlerDataResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to delete files from vector service: %v", err),
		}, nil
	}
	return &pb.DeleteCrawlerDataResponse{
		Success: true,
		Message: "Crawler data deleted successfully",
	}, nil
}

// deleteFilesFromVector deletes all files for a user from the vector service
func (s *crawlingServer) deleteFilesFromVector(ctx context.Context, userID string, platform string) error {
	platformEnum := convertPlatformToEnum(platform)
	request := &pb.VectorFileDeleteRequest{
		UserId:    userID,
		Platform:  platformEnum,
		Exclusive: true,
		Files:     []string{},
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err := s.vectorService.DeleteFiles(timeoutCtx, request)
	if err != nil {
		return fmt.Errorf("failed to delete files from vector service: %v", err)
	}

	return nil
}

func (s *crawlingServer) connectToTextChunkKafkaWriter() error {
	broker, ok := os.LookupEnv("KAFKA_BROKER_ADDRESS")
	if !ok {
		return fmt.Errorf("failed to retrieve kafka broker address")
	}

	s.kafkaWriter = &kafka.Writer{
		Addr:     kafka.TCP(broker),
		Topic:    "text-chunks",
		Balancer: &kafka.LeastBytes{},
	}

	return nil
}

func (s *crawlingServer) convertToProtoMetadata(metadata Metadata) *pb.Metadata {
	return &pb.Metadata{
		DateCreated:      timestamppb.New(metadata.DateCreated),
		DateLastModified: timestamppb.New(metadata.DateLastModified),
		UserId:           metadata.UserID,
		FilePath:         metadata.FilePath,
		Title:            metadata.Title,
		Platform:         convertPlatformToEnum(metadata.Platform),
		FileId:           metadata.ResourceID,
		ResourceType:     metadata.ResourceType,
		FileUrl:          metadata.FileURL,
		ChunkId:          metadata.ChunkID,
		Service:          metadata.Service,
	}

}

func (s *crawlingServer) sendChunkToVector(ctx context.Context, chunk TextChunkMessage) error {
	protoChunk := &pb.TextChunkMessage{
		Metadata: s.convertToProtoMetadata(chunk.Metadata),
		Content:  chunk.Content,
	}

	data, err := proto.Marshal(protoChunk)
	if err != nil {
		return fmt.Errorf("failed to serialize chunk: %v", err)
	}

	err = s.kafkaWriter.WriteMessages(ctx, kafka.Message{
		Value: data,
	})
	if err != nil {
		return fmt.Errorf("failed to write message to kafka: %v", err)
	}

	return nil
}

func (s *crawlingServer) sendFileDoneSignal(ctx context.Context, userID, filePath string, platform string) error {
	doneChunk := &pb.TextChunkMessage{
		Metadata: &pb.Metadata{
			UserId:   userID,
			FilePath: filePath,
			Platform: convertPlatformToEnum(platform),
		},
		Content: "<file_done>",
	}

	data, err := proto.Marshal(doneChunk)
	if err != nil {
		return fmt.Errorf("failed to serialize file done signal: %w", err)
	}

	err = s.kafkaWriter.WriteMessages(ctx, kafka.Message{
		Value: data,
	})
	if err != nil {
		return fmt.Errorf("failed to write file done signal: %w", err)
	}

	return nil
}

func (s *crawlingServer) sendCrawlDoneSignal(ctx context.Context, userID string, platform string) error {
	doneChunk := &pb.TextChunkMessage{
		Metadata: &pb.Metadata{
			UserId:   userID,
			Platform: convertPlatformToEnum(platform),
		},
		Content: "<crawl_done>",
	}

	data, err := proto.Marshal(doneChunk)
	if err != nil {
		return fmt.Errorf("failed to serialize crawl done signal: %w", err)
	}

	err = s.kafkaWriter.WriteMessages(ctx, kafka.Message{
		Value: data,
	})
	if err != nil {
		return fmt.Errorf("failed to write crawl done signal: %w", err)
	}

	return nil
}

func (s *crawlingServer) startCrawlingSignalReading(ctx context.Context) error {
	broker, ok := os.LookupEnv("KAFKA_BROKER_ADDRESS")
	if !ok {
		return fmt.Errorf("failed to retrieve kafka broker address")
	}

	googleReader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{broker},
		GroupID:  "google-crawling-signal-readers",
		Topic:    "google-crawling-signals",
		MaxBytes: 10e6,
	})
	defer googleReader.Close()

	notionReader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{broker},
		GroupID:  "notion-crawling-signal-readers",
		Topic:    "notion-crawling-signals",
		MaxBytes: 10e6,
	})
	defer notionReader.Close()

	microsoftReader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{broker},
		GroupID:  "microsoft-crawling-signal-readers",
		Topic:    "microsoft-crawling-signals",
		MaxBytes: 10e6,
	})
	defer microsoftReader.Close()

	messageCh := make(chan kafka.Message)
	errorCh := make(chan error)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				msg, err := googleReader.ReadMessage(ctx)
				if err != nil {
					errorCh <- err
				} else {
					messageCh <- msg
				}
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				msg, err := notionReader.ReadMessage(ctx)
				if err != nil {
					errorCh <- err
				} else {
					messageCh <- msg
				}
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				msg, err := microsoftReader.ReadMessage(ctx)
				if err != nil {
					errorCh <- err
				} else {
					messageCh <- msg
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errorCh:
			if err != nil {
				log.Printf("encountered error while reading from crawling signals kafka stream: %v", err)
			}
		case msg := <-messageCh:
			var signal pb.FileDoneProcessing
			if err := proto.Unmarshal(msg.Value, &signal); err != nil {
				log.Printf("Error unmarshalling message: %v", err)
				continue
			}

			var platform string
			switch msg.Topic {
			case "google-crawling-signals":
				platform = "GOOGLE"
			case "notion-crawling-signals":
				platform = "NOTION"
			case "microsoft-crawling-signals":
				platform = "MICROSOFT"
			default:
				continue
			}

			if signal.CrawlingDone {
				s.markCrawlingComplete(signal.UserId, platform)
			} else {
				s.markFileProcessed(signal.UserId, signal.FilePath, platform)
			}
		}
	}
}

func (s *crawlingServer) connectToVectorService(tlsConfig *tls.Config) {
	// Connect to the vector service
	vectorAddy, ok := os.LookupEnv("VECTOR_ADDRESS")
	if !ok {
		log.Fatal("failed to retrieve vector address for connection")
	}
	vectorConn, err := grpc.NewClient(
		vectorAddy,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
	)
	if err != nil {
		log.Fatalf("Failed to establish connection with vector-service: %v", err)
	}

	s.vectorConn = vectorConn
	s.vectorService = pb.NewVectorServiceClient(vectorConn)
}

func (s *crawlingServer) connectToIntegrationService(tlsConfig *tls.Config) {
	integrationAddress := os.Getenv("INTEGRATION_ADDRESS")
	if integrationAddress == "" {
		log.Fatalf("INTEGRATION_ADDRESS environment variable is required")
	}
	integrationConn, err := grpc.NewClient(
		integrationAddress,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
	)
	if err != nil {
		log.Fatalf("Failed to connect to integration service: %v", err)
	}

	s.integrationConn = integrationConn
	s.integrationService = pb.NewIntegrationServiceClient(integrationConn)
}

func (s *crawlingServer) connectToCouchDB(ctx context.Context) {
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
	chunkIDDBName := "chunk_ids"

	client, err := kivik.New("couch", fmt.Sprintf("http://%s:%s@%s/", couchdbUser, couchdbPassword, couchdbAddress))
	if err != nil {
		log.Fatalf("failed to connect to couchdb: %v", err)
	}
	s.CouchdbClient = client

	exists, err := client.DBExists(ctx, chunkIDDBName)
	if err != nil {
		log.Fatalf("failed to check if database exists: %v", err)
	} else if !exists {
		if err := client.CreateDB(ctx, chunkIDDBName); err != nil {
			log.Fatalf("failed to create chunk_ids couchdb database: %v", err)
		}
	}

	s.ChunkIDdb = client.DB(chunkIDDBName)
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Println("Starting the crawling service...")

	// Load the .env file
	err := config.LoadSharedConfig()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// Load the TLS configuration values for integration service
	clientTLSConfig, err := config.LoadClientTLSFromEnv("CRAWLING_CRT", "CRAWLING_KEY", "CA_CRT")
	if err != nil {
		log.Fatal("Error loading TLS client config for integration service")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatalf("DATABASE_URL environment variable is required")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := setupDatabase(db); err != nil {
		log.Fatalf("Database setup failed: %v", err)
	}

	grpcAddress := os.Getenv("CRAWLING_PORT")
	if grpcAddress == "" {
		log.Fatalf("CRAWLING_PORT environment variable is required")
	}
	tlsConfig, err := config.LoadServerTLSFromEnv("CRAWLING_CRT", "CRAWLING_KEY")
	if err != nil {
		log.Fatalf("Error loading TLS config for crawling service: %v", err)
	}
	listener, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	log.Println("Creating the crawling server")

	// Initialize the crawling server
	server := &crawlingServer{
		db: db,
	}

	server.connectToVectorService(clientTLSConfig)
	defer server.vectorConn.Close()

	server.connectToIntegrationService(clientTLSConfig)
	defer server.integrationConn.Close()
	// Connect to Kafka writer
	if err := server.connectToTextChunkKafkaWriter(); err != nil {
		log.Fatalf("Failed to connect to Kafka writer: %v", err)
	}
	defer server.kafkaWriter.Close()

	// Start the crawling signal reader
	go server.startCrawlingSignalReading(ctx)

	// Connect to chunk id database
	server.connectToCouchDB(ctx)
	defer server.ChunkIDdb.Close()

	// Start the periodic crawler worker
	server.startPeriodicCrawlerWorker(ctx)

	opts := []grpc.ServerOption{
		grpc.Creds(credentials.NewTLS(tlsConfig)),
	}
	grpcServer := grpc.NewServer(opts...)
	pb.RegisterCrawlingServiceServer(grpcServer, server)
	log.Printf("Crawling Service listening on %v\n", listener.Addr())
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	} else {
		log.Printf("Crawling Service served on %v\n", listener.Addr())
	}
}
