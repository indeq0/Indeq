package main

import (
	"context"
	"crypto/tls"
	"database/sql"
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
	"github.com/cc-0000/indeq/common/redis"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type params struct {
	memory      uint32 // default 64 KiB
	iterations  uint32 // default 3
	parallelism uint8  // default 2
	saltLength  uint32 // default 16
	keyLength   uint32 // default 32
}

type authServer struct {
	pb.UnimplementedAuthenticationServiceServer
	db                *sql.DB // password database
	desktopConn       *grpc.ClientConn
	desktopClient     pb.DesktopServiceClient
	integrationConn    *grpc.ClientConn
	integrationService pb.IntegrationServiceClient
	queryConn         *grpc.ClientConn
	queryService      pb.QueryServiceClient
	jwtSecret         []byte // secret for creating jwts
	argonParams       *params
	MinPasswordLength int
	MaxPasswordLength int
	MaxEmailLength    int
	redisClient       *redis.RedisClient
}


// TODO: implement rate limiting here
func (s *authServer) checkRateLimit(ctx context.Context, email string) (bool, error) {
	return false, nil
}

// TODO: implement failed attempts tracking here
func (s *authServer) incrementFailedAttempts(ctx context.Context, email string) error {
	return nil
}

// TODO: implement resetting the counter here
func (s *authServer) resetFailedAttempts(ctx context.Context, email string) error {
	return nil
}

// func() error
//   - loads the necessary parameters required by argon2, like memory, salt length, etc. into memory
//   - fallbacks to defaults if the parameters aren't present; if this isn't possible, an error will be returned
func (s *authServer) loadPasswordSettings() error {
	// Load parameters with defaults
	memory, err := strconv.ParseUint(os.Getenv("ARGON2_MEMORY"), 10, 32)
	if err != nil {
		memory = 64 * 1024 // default
	}

	iterations, err := strconv.ParseUint(os.Getenv("ARGON2_ITERATIONS"), 10, 32)
	if err != nil {
		iterations = 3 // default
	}

	parallelism, err := strconv.ParseUint(os.Getenv("ARGON2_PARALLELISM"), 10, 8)
	if err != nil {
		parallelism = 2 // default
	}

	saltLength, err := strconv.ParseUint(os.Getenv("ARGON2_SALT_LENGTH"), 10, 32)
	if err != nil {
		saltLength = 16 // default
	}

	keyLength, err := strconv.ParseUint(os.Getenv("ARGON2_KEY_LENGTH"), 10, 32)
	if err != nil {
		keyLength = 32 // default
	}

	s.argonParams = &params{
		memory:      uint32(memory),
		iterations:  uint32(iterations),
		parallelism: uint8(parallelism),
		saltLength:  uint32(saltLength),
		keyLength:   uint32(keyLength),
	}

	// Load constraints with defaults
	s.MinPasswordLength, err = strconv.Atoi(os.Getenv("MIN_PASSWORD_LENGTH"))
	if err != nil || s.MinPasswordLength <= 0 {
		s.MinPasswordLength = 8 // default
	}

	s.MaxPasswordLength, err = strconv.Atoi(os.Getenv("MAX_PASSWORD_LENGTH"))
	if err != nil || s.MaxPasswordLength <= 0 {
		s.MaxPasswordLength = 72 // default
	}

	s.MaxEmailLength, err = strconv.Atoi(os.Getenv("MAX_EMAIL_LENGTH"))
	if err != nil || s.MaxEmailLength <= 0 {
		s.MaxEmailLength = 255 // default
	}

	if _, ok := os.LookupEnv("JWT_SECRET"); !ok {
		return fmt.Errorf("JWT_SECRET environment variable is required")
	}
	s.jwtSecret = []byte(os.Getenv("JWT_SECRET"))

	return nil
}

// func(context, maximum amount of time it should take to connect to the database)
//   - connects to the database and creates the users table if necessary
//   - assumes: you will close the database connection elsewhere in the parent function(s)
func (s *authServer) connectToDatabase(ctx context.Context, contextDuration time.Duration) {
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

	// set up database table
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		log.Fatalf("failed to begin transaction after connecting to database: %v", err)
	}
	defer tx.Rollback()

	if _, err = tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS users (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            email VARCHAR(255) UNIQUE NOT NULL,
            password_hash TEXT NOT NULL,
            name VARCHAR(255) NOT NULL,
			alias VARCHAR(255) NOT NULL,
			avatar_num INT NOT NULL DEFAULT 1,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
        );
	`); err != nil {
		log.Fatalf("failed to create tables: %v", err)
	}

	// commit the transaction (1 in this case)
	if err = tx.Commit(); err != nil {
		log.Fatalf("failed to commit transaction: %v", err)
	}

	s.db = db
}

// func(client TLS config)
//   - connects to the desktop service using the provided client tls config and saves the connection and function interface to the server struct
//   - assumes: the connection will be closed in the parent function at some point
func (s *authServer) connectToDesktopService(tlsConfig *tls.Config) {
	// Connect to the desktop service
	desktopAddy, ok := os.LookupEnv("DESKTOP_ADDRESS")
	if !ok {
		log.Fatal("failed to retrieve desktop address for connection")
	}
	desktopConn, err := grpc.NewClient(
		desktopAddy,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
	)
	if err != nil {
		log.Fatalf("Failed to establish connection with desktop-service: %v", err)
	}

	s.desktopConn = desktopConn
	s.desktopClient = pb.NewDesktopServiceClient(desktopConn)
}

// func(client TLS config)
//   - connects to the integration service using the provided client tls config and saves the connection and function interface to the server struct
//   - assumes: the connection will be closed in the parent function at some point
func (s *authServer) connectToIntegrationService(tlsConfig *tls.Config) {
	// Connect to the integration service
	integrationAddy, ok := os.LookupEnv("INTEGRATION_ADDRESS")
	if !ok {
		log.Fatal("failed to retrieve integration address for connection")
	}
	integrationConn, err := grpc.NewClient(
		integrationAddy,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
	)
	if err != nil {
		log.Fatalf("Failed to establish connection with integration-service: %v", err)
	}

	s.integrationConn = integrationConn
	s.integrationService = pb.NewIntegrationServiceClient(integrationConn)
}

// func(client TLS config)
//   - connects to the query service using the provided client tls config and saves the connection and function interface to the server struct
//   - assumes: the connection will be closed in the parent function at some point
func (s *authServer) connectToQueryService(tlsConfig *tls.Config) {
	// Connect to the query service
	queryAddy, ok := os.LookupEnv("QUERY_ADDRESS")
	if !ok {
		log.Fatal("failed to retrieve query address for connection")
	}
	queryConn, err := grpc.NewClient(
		queryAddy,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
	)
	if err != nil {
		log.Fatalf("Failed to establish connection with query-service: %v", err)
	}

	s.queryConn = queryConn
	s.queryService = pb.NewQueryServiceClient(queryConn)
}

// func()
//   - sets up the gRPC server, connects it with the global struct, and TLS
//   - assumes: you will call grpcServer.GracefulStop() in the parent function at some point
func (s *authServer) createGRPCServer() *grpc.Server {
	// set up TLS for the gRPC server and serve it
	tlsConfig, err := config.LoadServerTLSFromEnv("AUTH_CRT", "AUTH_KEY")
	if err != nil {
		log.Fatalf("Error loading TLS config for authentication service: %v", err)
	}

	opts := []grpc.ServerOption{
		grpc.Creds(credentials.NewTLS(tlsConfig)),
	}
	grpcServer := grpc.NewServer(opts...)
	pb.RegisterAuthenticationServiceServer(grpcServer, s)

	return grpcServer
}

// func(pointer to a fully set up grpc server)
//   - starts the authentication-service grpc server
//   - this is a blocking call
func (s *authServer) startGRPCServer(grpcServer *grpc.Server) {
	grpcAddress, ok := os.LookupEnv("AUTH_PORT")
	if !ok {
		log.Fatal("failed to find the authentication service port in env variables")
	}

	listener, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()
	log.Printf("Authentication gRPC Service listening on %v\n", listener.Addr())

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

	// Load all .env variables
	err := config.LoadSharedConfig()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	// create the clientTLSConfig for use in connecting to other services
	clientTlsConfig, err := config.LoadClientTLSFromEnv("AUTH_CRT", "AUTH_KEY", "CA_CRT")
	if err != nil {
		log.Fatalf("failed to load client TLS configuration from .env: %v", err)
	}

	// create the server struct
	server := &authServer{}

	// connect to redis
	redisClient, err := redis.NewRedisClient(ctx, 1)
	if err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	}
	defer redisClient.Client.Close()
	server.redisClient = redisClient

	// load password settings
	if err := server.loadPasswordSettings(); err != nil {
		log.Fatalf("Failed to initialize password encryption settings: %v", err)
	}

	// start grpc server
	grpcServer := server.createGRPCServer()
	go server.startGRPCServer(grpcServer)
	defer grpcServer.GracefulStop()

	// connect to database
	server.connectToDatabase(ctx, 10*time.Second)
	defer server.db.Close()

	// Connect to the desktop service
	server.connectToDesktopService(clientTlsConfig)
	defer server.desktopConn.Close()

	// Connect to the integration service
	server.connectToIntegrationService(clientTlsConfig)
	defer server.integrationConn.Close()

	// Connect to the query service
	server.connectToQueryService(clientTlsConfig)
	defer server.queryConn.Close()

	<-sigChan // TODO: implement worker groups
	log.Print("gracefully shutting down...")
}
