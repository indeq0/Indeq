package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	pb "github.com/cc-0000/indeq/common/api"

	"github.com/cc-0000/indeq/common/config"
	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/proto"

	"github.com/segmentio/kafka-go"
)

type vectorServer struct {
	pb.UnimplementedVectorServiceServer
	milvusClient            client.Client
	embeddingConn           *grpc.ClientConn
	embeddingClient         pb.EmbeddingServiceClient
	desktopWriter           *kafka.Writer
	googleCrawlingWriter    *kafka.Writer
	notionCrawlingWriter    *kafka.Writer
	microsoftCrawlingWriter *kafka.Writer
	collectionName          string
}

// func(context):
//   - starts a kafka reader on the text-chunks topic to read in text chunks that need processing
//   - generates embeddings for the text chunks and sends signals back to the correct topic signalling processing is done
//   - this is a blocking call
//   - assumes: you will catch any errors and deal with it in th parent function
func (s *vectorServer) startTextChunkProcess(ctx context.Context) error {
	// create a kafka reader to read ALL incoming chunks, regardless of origin
	broker, ok := os.LookupEnv("KAFKA_BROKER_ADDRESS")
	if !ok {
		return fmt.Errorf("failed to retrieve kafka broker address")
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{broker},
		GroupID:  "vector-readers", // other nodes can also join this consumer group
		Topic:    "text-chunks",
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

	batchSize, err := strconv.Atoi(os.Getenv("VECTOR_BATCH_SIZE"))
	if err != nil {
		return fmt.Errorf("failed to retrieve env variable vector batch size: %w", err)
	}

	timeoutNum, err := strconv.Atoi(os.Getenv("VECTOR_BATCH_TIMEOUT"))
	if err != nil {
		return fmt.Errorf("failed to retrieve env variable vector batch timeout: %w", err)
	}
	timeout := time.Duration(timeoutNum) * time.Second

	batch := make([]*pb.TextChunkMessage, 0, batchSize)
	var timerCh <-chan time.Time

	// here we check the channels
	for {
		// we have a message in the batch so we start counting down
		if len(batch) > 0 && timerCh == nil {
			timerCh = time.After(timeout)
		}

		select {
		case <-ctx.Done():
			log.Print("Shutting down text chunk consumer...")
			return nil
		default:
			select {
			// timer went off so we want to send the batch
			case <-timerCh:
				err := s.processBatch(ctx, batch)
				if err != nil {
					log.Printf("failed to process batch after timer: %v", err)
					continue
				} else {
					batch = batch[:0]
					timerCh = nil
				}
			// an error was detected; we want to log this
			case err := <-errorCh:
				if err != nil {
					log.Printf("encountered error while reading from text chunk kafka stream: %v", err)
					continue
				}
			// a message was detected; based on the content, we will do something...
			case msg := <-messageCh:
				var textChunk pb.TextChunkMessage
				if err := proto.Unmarshal(msg.Value, &textChunk); err != nil {
					log.Printf("Error unmarshalling message: %v", err)
					continue
				}

				// check if file is done or if crawling is done, or if we're just processing a text chunk
				if textChunk.Content == "<file_done>" {
					// Process batch before handling file_done signal
					err := s.processBatch(ctx, batch)
					if err != nil {
						log.Printf("failed to process batch after file_done signal: %v", err)
						continue
					} else {
						batch = batch[:0]
						timerCh = nil
					}

					if textChunk.Metadata == nil {
						log.Print("no metadata detected for this text chunk, aborting the file_done signal")
						continue
					}
					// Write the message to the proper stream to let them know we are done
					if textChunk.Metadata.Platform == pb.Platform_PLATFORM_LOCAL {
						fileDoneMessage := &pb.FileDoneProcessing{
							UserId:   textChunk.Metadata.UserId,
							FilePath: textChunk.Metadata.FilePath,
						}
						byteMessage, err := proto.Marshal(fileDoneMessage)
						if err != nil {
							log.Print("failed to serialized file done message: ", err)
							continue
						}
						message := kafka.Message{
							Value: byteMessage,
						}

						if err := s.desktopWriter.WriteMessages(ctx, message); err != nil {
							log.Print("error: ", err)
							continue
						}
					} else if textChunk.Metadata.Platform == pb.Platform_PLATFORM_GOOGLE {
						fileDoneMessage := &pb.FileDoneProcessing{
							UserId:   textChunk.Metadata.UserId,
							FilePath: textChunk.Metadata.FilePath,
						}
						byteMessage, err := proto.Marshal(fileDoneMessage)
						if err != nil {
							log.Print("failed to serialized file done message: ", err)
							continue
						}
						message := kafka.Message{
							Value: byteMessage,
						}

						if err := s.googleCrawlingWriter.WriteMessages(ctx, message); err != nil {
							log.Print("error: ", err)
							continue
						}
					} else if textChunk.Metadata.Platform == pb.Platform_PLATFORM_NOTION {
						fileDoneMessage := &pb.FileDoneProcessing{
							UserId:   textChunk.Metadata.UserId,
							FilePath: textChunk.Metadata.FilePath,
						}
						byteMessage, err := proto.Marshal(fileDoneMessage)
						if err != nil {
							log.Print("failed to serialized file done message: ", err)
							continue
						}
						message := kafka.Message{
							Value: byteMessage,
						}
						if err := s.notionCrawlingWriter.WriteMessages(ctx, message); err != nil {
							log.Print("error: ", err)
							continue
						}
					} else if textChunk.Metadata.Platform == pb.Platform_PLATFORM_MICROSOFT {
						fileDoneMessage := &pb.FileDoneProcessing{
							UserId:   textChunk.Metadata.UserId,
							FilePath: textChunk.Metadata.FilePath,
						}
						byteMessage, err := proto.Marshal(fileDoneMessage)
						if err != nil {
							log.Print("failed to serialized file done message: ", err)
							continue
						}
						message := kafka.Message{
							Value: byteMessage,
						}
						if err := s.microsoftCrawlingWriter.WriteMessages(ctx, message); err != nil {
							log.Print("error: ", err)
							continue
						}
					}

					// TODO: send the signals to other platform topics
				} else if textChunk.Content == "<crawl_done>" {
					if textChunk.Metadata == nil {
						log.Print("no metadata detected for this textchunk")
						continue
					}

					if textChunk.Metadata.Platform == pb.Platform_PLATFORM_LOCAL {
						fileDoneMessage := &pb.FileDoneProcessing{
							UserId:       textChunk.Metadata.UserId,
							CrawlingDone: true,
						}
						byteMessage, err := proto.Marshal(fileDoneMessage)
						if err != nil {
							log.Print("failed to serialized file done message: ", err)
							continue
						}
						message := kafka.Message{
							Value: byteMessage,
						}

						if err := s.desktopWriter.WriteMessages(ctx, message); err != nil {
							log.Print("error: ", err)
							continue
						}
					} else if textChunk.Metadata.Platform == pb.Platform_PLATFORM_GOOGLE {
						fileDoneMessage := &pb.FileDoneProcessing{
							UserId:       textChunk.Metadata.UserId,
							CrawlingDone: true,
						}
						byteMessage, err := proto.Marshal(fileDoneMessage)
						if err != nil {
							log.Print("failed to serialized file done message: ", err)
							continue
						}
						message := kafka.Message{
							Value: byteMessage,
						}

						if err := s.googleCrawlingWriter.WriteMessages(ctx, message); err != nil {
							log.Print("error: ", err)
							continue
						}
					} else if textChunk.Metadata.Platform == pb.Platform_PLATFORM_NOTION {
						fileDoneMessage := &pb.FileDoneProcessing{
							UserId:       textChunk.Metadata.UserId,
							CrawlingDone: true,
						}
						byteMessage, err := proto.Marshal(fileDoneMessage)
						if err != nil {
							log.Print("failed to serialized file done message: ", err)
							continue
						}
						message := kafka.Message{
							Value: byteMessage,
						}

						if err := s.notionCrawlingWriter.WriteMessages(ctx, message); err != nil {
							log.Print("error: ", err)
							continue
						}
					} else if textChunk.Metadata.Platform == pb.Platform_PLATFORM_MICROSOFT {
						fileDoneMessage := &pb.FileDoneProcessing{
							UserId:       textChunk.Metadata.UserId,
							CrawlingDone: true,
						}
						byteMessage, err := proto.Marshal(fileDoneMessage)
						if err != nil {
							log.Print("failed to serialized file done message: ", err)
							continue
						}
						message := kafka.Message{
							Value: byteMessage,
						}

						if err := s.microsoftCrawlingWriter.WriteMessages(ctx, message); err != nil {
							log.Print("error: ", err)
							continue
						}
					}
					// TODO: send the signals to other platform topics
				} else {
					// Add the textchunk to the batch
					batch = append(batch, &textChunk)

					// If batch is full, process it
					if len(batch) >= batchSize {
						err := s.processBatch(ctx, batch)
						if err != nil {
							log.Printf("failed to process batch after overflow: %v", err)
							continue
						} else {
							batch = batch[:0]
							timerCh = nil
						}
					}
				}
			}
		}
	}
}

// func(client TLS config)
//   - connects to the embedding service using the provided client tls config and saves the connection and function interface to the server struct
//   - assumes: the connection will be closed in the parent function at some point
func (s *vectorServer) connectToEmbeddingService(tlsConfig *tls.Config) {
	// connect to the embedding service
	embeddingAddy, ok := os.LookupEnv("EMBEDDING_ADDRESS")
	if !ok {
		log.Fatal("failed to retrieve desktop address for connection")
	}
	embeddingConn, err := grpc.NewClient(
		embeddingAddy,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
	)
	if err != nil {
		log.Fatalf("Failed to establish connection with embedding-service: %v", err)
	}

	s.embeddingConn = embeddingConn
	s.embeddingClient = pb.NewEmbeddingServiceClient(embeddingConn)
}

// func()
//   - creates a kafka writer interface for writing desktop signals like (file_done, crawl_done, etc.)
//   - assumes: that a topic called 'desktop-signals' has already been created elsewhere (like in an init routine)
//   - assumes: you will close the writer elsewhere in the parent once you are done using it
func (s *vectorServer) createDesktopWriter() {
	broker, ok := os.LookupEnv("KAFKA_BROKER_ADDRESS")
	if !ok {
		log.Fatal("failed to retrieve kafka broker address")
	}

	desktopWriter := &kafka.Writer{
		Addr:         kafka.TCP(broker),
		Topic:        "desktop-signals",   // this writer is specifically directed towards desktop signals (file_done, crawl_done, etc.)
		Balancer:     &kafka.LeastBytes{}, // routes to the least congested partition
		BatchSize:    10,
		BatchTimeout: 1 * time.Second,
		Compression:  kafka.Lz4,
		Async:        true, // will not wait for the batch timeout to send messages
		Completion: func(messages []kafka.Message, err error) {
			if err != nil {
				log.Printf("encountered error while writing message to desktop-signals: %v", err)
			}
		},
	}
	s.desktopWriter = desktopWriter
}

// func()
//   - creates a kafka writer interface for writing crawling signals like (crawl_done, etc.)
//   - assumes: that a topic called 'crawling-signals' has already been created elsewhere (like in an init routine)
//   - assumes: you will close the writer elsewhere in the parent once you are done using it
func (s *vectorServer) createGoogleCrawlingWriter() {
	broker, ok := os.LookupEnv("KAFKA_BROKER_ADDRESS")
	if !ok {
		log.Fatal("failed to retrieve kafka broker address")
	}

	googleCrawlingWriter := &kafka.Writer{
		Addr:         kafka.TCP(broker),
		Topic:        "google-crawling-signals",
		Balancer:     &kafka.LeastBytes{},
		BatchSize:    10,
		BatchTimeout: 1 * time.Second,
		Compression:  kafka.Lz4,
		Async:        true, // will not wait for the batch timeout to send messages
		Completion: func(messages []kafka.Message, err error) {
			if err != nil {
				log.Printf("encountered error while writing message to google-crawling-signals: %v", err)
			}
		},
	}
	s.googleCrawlingWriter = googleCrawlingWriter
}

func (s *vectorServer) createNotionCrawlingWriter() {
	broker, ok := os.LookupEnv("KAFKA_BROKER_ADDRESS")
	if !ok {
		log.Fatal("failed to retrieve kafka broker address")
	}

	notionCrawlingWriter := &kafka.Writer{
		Addr:         kafka.TCP(broker),
		Topic:        "notion-crawling-signals",
		Balancer:     &kafka.LeastBytes{},
		BatchSize:    10,
		BatchTimeout: 1 * time.Second,
		Compression:  kafka.Lz4,
		Async:        true, // will not wait for the batch timeout to send messages
		Completion: func(messages []kafka.Message, err error) {
			if err != nil {
				log.Printf("encountered error while writing message to notion-crawling-signals: %v", err)
			}
		},
	}
	s.notionCrawlingWriter = notionCrawlingWriter
}

func (s *vectorServer) createMicrosoftCrawlingWriter() {
	broker, ok := os.LookupEnv("KAFKA_BROKER_ADDRESS")
	if !ok {
		log.Fatal("failed to retrieve kafka broker address")
	}

	microsoftCrawlingWriter := &kafka.Writer{
		Addr:         kafka.TCP(broker),
		Topic:        "microsoft-crawling-signals",
		Balancer:     &kafka.LeastBytes{},
		BatchSize:    10,
		BatchTimeout: 1 * time.Second,
		Compression:  kafka.Lz4,
		Async:        true, // will not wait for the batch timeout to send messages
		Completion: func(messages []kafka.Message, err error) {
			if err != nil {
				log.Printf("encountered error while writing message to microsoft-crawling-signals: %v", err)
			}
		},
	}
	s.microsoftCrawlingWriter = microsoftCrawlingWriter
}

// func(context)
//   - connects to the milvus instance using an api key from .env variables
//   - assumes: the client will be closed in the parent function some point
func (s *vectorServer) connectToMilvus(ctx context.Context) {
	// Configure keepalive parameters to prevent "too_many_pings" errors
	kacp := keepalive.ClientParameters{
		Time:                20 * time.Second, // Send pings every 20 seconds if there is no activity
		Timeout:             10 * time.Second, // Wait 10 seconds for ping ack before considering the connection dead
		PermitWithoutStream: false,            // Don't send pings without active streams
	}
	// Create custom dial options
	dialOpts := []grpc.DialOption{
		grpc.WithKeepaliveParams(kacp),
	}

	milvusClient, err := client.NewClient(ctx, client.Config{
		Address:     os.Getenv("ZILLIZ_ADDRESS"),
		APIKey:      os.Getenv("ZILLIZ_API_KEY"),
		DialOptions: dialOpts,
	})
	if err != nil {
		log.Fatalf("failed to connect to the milvus instance: %v", err)
	}

	s.milvusClient = milvusClient
}

// func()
//   - sets up the gRPC server, connects it with the global struct, and TLS
//   - assumes: you will call grpcServer.GracefulStop() in the parent function at some point
func (s *vectorServer) createGRPCServer() *grpc.Server {
	// set up TLS for the gRPC server and serve it
	tlsConfig, err := config.LoadServerTLSFromEnv("VECTOR_CRT", "VECTOR_KEY")
	if err != nil {
		log.Fatalf("Error loading TLS config for vector service: %v", err)
	}

	opts := []grpc.ServerOption{
		grpc.Creds(credentials.NewTLS(tlsConfig)),
	}
	grpcServer := grpc.NewServer(opts...)
	pb.RegisterVectorServiceServer(grpcServer, s)

	return grpcServer
}

// func(pointer to a fully set up grpc server)
//   - starts the vector-service grpc server
//   - this is a blocking call
func (s *vectorServer) startGRPCServer(grpcServer *grpc.Server) {
	grpcAddress, ok := os.LookupEnv("VECTOR_PORT")
	if !ok {
		log.Fatal("failed to find the vector service port in env variables")
	}

	listener, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()
	log.Printf("Vector gRPC Service listening on %v\n", listener.Addr())

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Load the .env file
	err := config.LoadSharedConfig()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	clientTlsConfig, err := config.LoadClientTLSFromEnv("VECTOR_CRT", "VECTOR_KEY", "CA_CRT")
	if err != nil {
		log.Fatal("Error loading TLS client config for vector service")
	}

	// initialize the vector server struct with a hardcoded collection name
	server := &vectorServer{
		collectionName: "collection_1",
	}

	// connect to milvus db
	server.connectToMilvus(ctx)
	defer server.milvusClient.Close()

	// set up our collection if need be
	if err := server.setupCollection(ctx, server.collectionName); err != nil {
		log.Fatalf("Error when setting up collection in our milvus database: %v", err)
	}

	// start grpc server
	grpcServer := server.createGRPCServer()
	go server.startGRPCServer(grpcServer)
	defer grpcServer.GracefulStop()

	// connect to the embedding gRPC service
	server.connectToEmbeddingService(clientTlsConfig)
	defer server.embeddingConn.Close()

	// connect to apache kafka
	server.createDesktopWriter()
	defer server.desktopWriter.Close()

	// connect to crawling kafka
	server.createGoogleCrawlingWriter()
	defer server.googleCrawlingWriter.Close()

	server.createNotionCrawlingWriter()
	defer server.notionCrawlingWriter.Close()

	server.createMicrosoftCrawlingWriter()
	defer server.microsoftCrawlingWriter.Close()

	// TODO: create other writers to signal for google drive, microsoft office, notion, etc.

	// start the kafka reader to process incoming chunks
	go func() {
		if err := server.startTextChunkProcess(ctx); err != nil {
			log.Printf("encountered error in text chunk goroutine: %v", err)
		}
	}()

	<-sigChan // TODO: implement worker groups
	log.Print("gracefully shutting down...")
}
