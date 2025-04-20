package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"math/rand"
	"sync"
	"syscall"

	"log"
	"net"
	"os"
	"os/signal"
	"time"

	pb "github.com/cc-0000/indeq/common/api"
	"github.com/cc-0000/indeq/common/config"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"
)

type desktopServer struct {
	pb.UnimplementedDesktopServiceServer
	vectorConn        *grpc.ClientConn
	vectorService     pb.VectorServiceClient
	mqttClient        mqtt.Client
	db                *sql.DB
	kafkaWriter       *kafka.Writer
	queryChannels     map[int32]chan *pb.DesktopChunkResponse
	queryChannelMutex sync.Mutex
}

// rpc(context, get chunk request)
//   - tries to get the requested chunks from the specified user_id by sending a list of metadatas
//   - times out and returns when ttl expires
//   - assumes: that each user will only be subscribed to their own topic/user_id
func (s *desktopServer) GetChunksFromUser(ctx context.Context, req *pb.GetChunksFromUserRequest) (*pb.GetChunksFromUserResponse, error) {
	// generate a pseudo-unique random request ID
	requestID := int32(time.Now().UnixNano()) + int32(rand.Intn(1000))
	userID := req.UserId
	timeout := time.Duration(int64(req.Ttl)) * time.Millisecond

	// create the req/res channel
	s.queryChannelMutex.Lock()
	s.queryChannels[requestID] = make(chan *pb.DesktopChunkResponse, 1)
	s.queryChannelMutex.Unlock()

	// send the message to the desktop client
	queryReq := &pb.DesktopChunkRequest{
		RequestId:       requestID,
		RequestedChunks: req.Metadatas,
	}
	payload, err := proto.Marshal(queryReq)
	if err != nil {
		return &pb.GetChunksFromUserResponse{
			NumChunks: 0,
			Chunks:    []*pb.TextChunkMessage{},
		}, fmt.Errorf("failed to serialize the crawl request: %v", err)
	}

	s.mqttClient.Publish(fmt.Sprintf("query_req/%s", userID), 2, false, payload)

	// wait for the response
	select {
	// received chunks from user in time
	case result := <-s.queryChannels[requestID]:
		s.queryChannelMutex.Lock()
		defer s.queryChannelMutex.Unlock()
		ch, exists := s.queryChannels[requestID]
		if exists {
			close(ch)
			delete(s.queryChannels, requestID)
		}
		return &pb.GetChunksFromUserResponse{
			NumChunks: int32(len(result.TextChunks)),
			Chunks:    result.TextChunks,
		}, nil
	// we ran out of time
	case <-time.After(timeout):
		s.queryChannelMutex.Lock()
		defer s.queryChannelMutex.Unlock()
		ch, exists := s.queryChannels[requestID]
		if exists {
			close(ch)
			delete(s.queryChannels, requestID)
		}
		return &pb.GetChunksFromUserResponse{
			NumChunks: 0,
			Chunks:    []*pb.TextChunkMessage{},
		}, fmt.Errorf("request timed out after %s milliseconds", timeout)
	}
}

// rpc(context, setup user stats request)
//   - sets up the default crawl_stats values for the given user
//   - assumes: the user just registered and is valid
func (s *desktopServer) SetupUserStats(ctx context.Context, req *pb.SetupUserStatsRequest) (*pb.SetupUserStatsResponse, error) {
	if err := s.createDefaultCrawlStatsEntry(ctx, req.UserId); err != nil {
		return &pb.SetupUserStatsResponse{
			Success: false,
		}, err
	}
	return &pb.SetupUserStatsResponse{
		Success: true,
	}, nil
}

// rpc(context, get crawl stats request)
//   - retrieves the crawl statistics for a given user
//   - returns the number of files crawled, total files, crawling status, and online status
//   - assumes: the user exists in the database
func (s *desktopServer) GetCrawlStats(ctx context.Context, req *pb.GetCrawlStatsRequest) (*pb.GetCrawlStatsResponse, error) {
	// Get the crawl statistics from the database
	crawledFiles, totalFiles, isCrawling, isOnline, err := s.getCrawlStats(ctx, req.UserId)
	if err != nil {
		return nil, fmt.Errorf("failed to get crawl stats: %v", err)
	}

	// Return the response with the crawl statistics
	return &pb.GetCrawlStatsResponse{
		CrawledFiles: crawledFiles,
		TotalFiles:   totalFiles,
		IsCrawling:   isCrawling,
		IsOnline:     isOnline,
	}, nil
}

// rpc(context, update user online status request)
//   - updates the online status for a given user in the crawl_stats table
//   - returns an error if the update fails, otherwise returns an empty response
//   - assumes: the user exists in the database
func (s *desktopServer) UpdateUserOnlineStatus(ctx context.Context, req *pb.UpdateUserOnlineStatusRequest) (*pb.UpdateUserOnlineStatusResponse, error) {
	// Update the user's online status in the database
	err := s.updateUserOnlineStatus(ctx, req.UserId, req.IsOnline)
	if err != nil {
		return nil, fmt.Errorf("failed to update online status: %v", err)
	}

	// Return empty response on success
	return &pb.UpdateUserOnlineStatusResponse{}, nil
}

// func(context, how often we want to check, how long can a crawl be running for before cancelled)
//   - checks every checkInterval for users for have been crawling for longer than allowedIdleTime and 'kills' those crawls
//   - assumes: you will cancel the context when you want to stop this process
func (s *desktopServer) startJobCheckDeadCrawling(ctx context.Context, checkInterval time.Duration, allowedIdleTime time.Duration) {
	// launch the goroutine
	go func() {
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// call the kill idle crawls function
				if err := s.killIdleCrawls(ctx, allowedIdleTime); err != nil {
					log.Printf("failed to check for idle crawls: %v", err)
				}
			}
		}
	}()
}

// func(context)
//   - blocking call
//   - reads desktop finish signals from the the kafka stream (when files or entire crawls are finished processing)
//   - sets them accordingly in the database
//   - assumes: processing has been completed elsewhere for the file(s)
func (s *desktopServer) startDesktopSignalReading(ctx context.Context) error {
	broker, ok := os.LookupEnv("KAFKA_BROKER_ADDRESS")
	if !ok {
		return fmt.Errorf("failed to retrieve kafka broker address")
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{broker},
		GroupID:  "desktop-signal-readers",
		Topic:    "desktop-signals",
		MaxBytes: 10e6, // maximum batch size 10MB
	})
	defer reader.Close()

	// start a goroutine to read the messages and pipe them into channels
	messageCh := make(chan kafka.Message)
	errorCh := make(chan error)

	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Print("Shutting down text chunk consumer channeler")
				return
			default:
				msg, err := reader.ReadMessage(ctx)
				if err != nil {
					errorCh <- err
				} else {
					messageCh <- msg
				}
			}
		}
	}()

	// here we check the channels
	for {
		select {
		case <-ctx.Done():
			log.Print("Shutting down text chunk consumer processor...")
			return nil
		case err := <-errorCh:
			if err != nil {
				log.Printf("encountered error while reading from desktop signals kafka stream: %v", err)
			}
		case msg := <-messageCh:
			var signal pb.FileDoneProcessing
			if err := proto.Unmarshal(msg.Value, &signal); err != nil {
				log.Printf("Error unmarshalling message: %v", err)
				continue
			}
			if signal.CrawlingDone {
				if err := s.markCrawlingDone(ctx, signal.UserId); err != nil {
					log.Printf("error marking crawling done for user %s: %v", signal.UserId, err)
					continue
				}
				log.Print("crawling done for user ", signal.UserId)
			} else {
				if err := s.markFileDone(ctx, signal.UserId, signal.FilePath); err != nil {
					log.Printf("error marking file as done for user %s: %v", signal.UserId, err)
					continue
				}
				log.Print("file done for user ", signal.UserId)
			}
		}
	}
}

// func(context, maximum amount of time it should take to connect to the database)
//   - connects to the database and creates the crawl_stats and indexed_files tables if necessary
//   - assumes: you will close the database connection elsewhere in the parent function(s)
func (s *desktopServer) connectToDatabase(ctx context.Context, contextDuration time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, contextDuration)
	defer cancel()

	// get env variables
	dbURL, ok := os.LookupEnv("DATABASE_URL")
	if !ok {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	// connect to database
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// set up database tables
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		log.Fatalf("failed to begin transaction after connecting to database: %v", err)
	}
	defer tx.Rollback()

	if _, err = tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS crawl_stats (
			user_id UUID PRIMARY KEY,
			online BOOLEAN NOT NULL,
			crawling BOOLEAN NOT NULL,
			crawled_files SMALLINT,
			total_files SMALLINT,
			updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS indexed_files (
			user_id UUID NOT NULL,
			file_path TEXT NOT NULL,
			hash VARCHAR(255) NOT NULL,
			done BOOLEAN
		);
	`); err != nil {
		log.Fatalf("failed to create tables: %v", err)
	}

	if _, err = tx.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS indexed_files_user_idx ON indexed_files(user_id);
		CREATE INDEX IF NOT EXISTS indexed_files_user_file_idx ON indexed_files(user_id, file_path);
	`); err != nil {
		log.Fatalf("failed to create indexes: %v", err)
	}

	if _, err = tx.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION refresh_crawling_updated_column()
		RETURNS TRIGGER AS $$
		BEGIN
			NEW.updated_at = NOW();
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;

		DROP TRIGGER IF EXISTS update_crawl_stats_updated_at ON crawl_stats;

		CREATE TRIGGER update_crawl_stats_updated_at
		BEFORE UPDATE ON crawl_stats
		FOR EACH ROW
		EXECUTE FUNCTION refresh_crawling_updated_column();
	`); err != nil {
		log.Fatalf("failed to set up triggers: %v", err)
	}

	if err = tx.Commit(); err != nil {
		log.Fatalf("failed to commit transaction: %v", err)
	}

	s.db = db
}

// func(client TLS config)
//   - connects to the MQTT broker via the client configuration (for mTLS) and subscribes to the necessary topics
//   - saves the client to the server struct for later use
//   - assumes: that you will close the connection elsewhere
func (s *desktopServer) connectToMQTTBroker(tlsConfig *tls.Config) {
	mqttAddress, ok := os.LookupEnv("MQTT_ADDRESS")
	if !ok {
		log.Fatal("failed to retrieve mqtt address")
	}
	clientID := "desktop_mqtt_client"

	// Connection handler
	connectHandler := func(client mqtt.Client) {
		log.Println("Connected to mqtt broker")
	}

	// Connection lost handler
	connectLostHandler := func(client mqtt.Client, err error) {
		log.Printf("Mqtt broker connection lost: %v\n", err)
	}

	// Create client options
	opts := mqtt.NewClientOptions()
	opts.SetTLSConfig(tlsConfig)
	opts.AddBroker(mqttAddress)
	opts.SetClientID(clientID)
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = connectLostHandler
	opts.SetAutoReconnect(true)

	// Create and start client
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Print(token.Error())
		panic(token.Error())
	}

	// Subscribe to topics
	topic := "new_crawl/+"
	if token := client.Subscribe(topic, 2, s.handleCrawlRequest); token.Wait() && token.Error() != nil {
		log.Fatal("failed to subscribe to topic ", topic, " : ", token.Error())
	}

	topic = "new_chunk/+"
	if token := client.Subscribe(topic, 2, s.handleChunk); token.Wait() && token.Error() != nil {
		log.Fatal("failed to subscribe to topic ", topic, " : ", token.Error())
	}

	topic = "query_res/+"
	if token := client.Subscribe(topic, 2, s.handleQueryResponse); token.Wait() && token.Error() != nil {
		log.Fatal("failed to subscribe to topic ", topic, " : ", token.Error())
	}

	s.mqttClient = client
}

// func(client TLS config)
//   - connects to the vector service using the provided client tls config and saves the connection and function interface to the server struct
//   - assumes: the connection will be closed in the parent function at some point
func (s *desktopServer) connectToVectorService(tlsConfig *tls.Config) {
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

// func()
//   - connects to the kafka broker and returns a writer to the text-chunks topic
//   - assumes: you will close the writer in the parent function at some point
func (s *desktopServer) connectToTextChunkKafkaWriter() {
	broker, ok := os.LookupEnv("KAFKA_BROKER_ADDRESS")
	if !ok {
		log.Fatal("failed to retrieve kafka broker address")
	}

	kafkaWriter := &kafka.Writer{
		Addr:         kafka.TCP(broker),
		Topic:        "text-chunks",
		Balancer:     &kafka.LeastBytes{}, // routes to the least congested partition
		BatchSize:    100,                 // maximum batch size before batch is sent
		BatchTimeout: 1 * time.Second,     // how often each batch is sent
		Compression:  kafka.Lz4,
		Async:        true,
		Completion: func(messages []kafka.Message, err error) {
			if err != nil {
				log.Printf("encountered error while writing message: %v", err)
			}
		},
	}

	s.kafkaWriter = kafkaWriter
}

// func()
//   - sets up the gRPC server, connects it with the global struct, and TLS
//   - assumes: you will call grpcServer.GracefulStop() in the parent function at some point
func (s *desktopServer) createGRPCServer() *grpc.Server {
	// set up TLS for the gRPC server and serve it
	tlsConfig, err := config.LoadServerTLSFromEnv("DESKTOP_CRT", "DESKTOP_KEY")
	if err != nil {
		log.Fatalf("Error loading TLS config for desktop service: %v", err)
	}

	opts := []grpc.ServerOption{
		grpc.Creds(credentials.NewTLS(tlsConfig)),
	}
	grpcServer := grpc.NewServer(opts...)
	pb.RegisterDesktopServiceServer(grpcServer, s)

	return grpcServer
}

// func(pointer to a fully set up grpc server)
//   - starts the desktop-service grpc server
//   - this is a blocking call
func (s *desktopServer) startGRPCServer(grpcServer *grpc.Server) {
	log.Println("Starting the desktop gRPC server...")

	grpcAddress, ok := os.LookupEnv("DESKTOP_PORT")
	if !ok {
		log.Fatal("failed to find the desktop service port in env variables")
	}

	listener, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()
	log.Printf("Desktop gRPC Service listening on %v\n", listener.Addr())

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load .env variables
	err := config.LoadSharedConfig()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// create the clientTLSConfig for use in connecting to other services
	clientTlsConfig, err := config.LoadClientTLSFromEnv("DESKTOP_CRT", "DESKTOP_KEY", "CA_CRT")
	if err != nil {
		log.Fatalf("failed to load client TLS configuration from .env: %v", err)
	}

	// create the server struct
	server := &desktopServer{
		queryChannels: make(map[int32]chan *pb.DesktopChunkResponse),
	}

	// Connect to the Desktop Database
	server.connectToDatabase(ctx, 10*time.Second)
	server.startJobCheckDeadCrawling(ctx, 5*time.Minute, 30*time.Minute)
	defer server.db.Close()
	defer markAllCrawlingDone(ctx, server.db)

	// Connect to Apache Kafka
	server.connectToTextChunkKafkaWriter()
	defer server.kafkaWriter.Close()

	// Connect to the MQTT Broker
	server.connectToMQTTBroker(clientTlsConfig)
	defer server.mqttClient.Disconnect(250)

	// Connect to the vector service
	server.connectToVectorService(clientTlsConfig)
	defer server.vectorConn.Close()

	// Start reading from the kafka stream
	go server.startDesktopSignalReading(ctx)

	// create and start the gRPC server
	grpcServer := server.createGRPCServer()
	go server.startGRPCServer(grpcServer)
	defer grpcServer.GracefulStop()

	<-sigChan // TODO: implement worker groups
	log.Print("gracefully shutting down...")
}
