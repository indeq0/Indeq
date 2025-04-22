package main

import (
	"context"
	"database/sql"
	"log"
	"net"
	"net/mail"
	"os"
	"time"

	pb "github.com/cc-0000/indeq/common/api"
	"github.com/cc-0000/indeq/common/config"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type WaitlistServer struct {
	pb.UnimplementedWaitlistServiceServer
	db *sql.DB // waitlist db
}

func (s *WaitlistServer) AddToWaitlist(ctx context.Context, req *pb.AddToWaitlistRequest) (*pb.AddToWaitlistResponse, error) {
	log.Println("Adding to waitlist:", req.Email)
	_, err := mail.ParseAddress(req.Email)
	if err != nil {
		return &pb.AddToWaitlistResponse{
			Success: false,
			Message: "Invalid email address",
		}, nil
	}

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO waitlist (email)
		VALUES ($1)
		ON CONFLICT (email) DO NOTHING`, req.Email)

	if err != nil {
		log.Println("Database insert error:", err)
		return &pb.AddToWaitlistResponse{
			Success: false,
			Message: "Could not add to waitlist. Please try again later.",
		}, nil
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Println("Error retrieving affected rows:", err)
		return &pb.AddToWaitlistResponse{
			Success: false,
			Message: "Could not verify waitlist status. Please try again later.",
		}, nil
	}

	if rowsAffected == 0 {
		return &pb.AddToWaitlistResponse{
			Success: false,
			Message: "You're already on the waitlist! 😊",
		}, nil
	}

	return &pb.AddToWaitlistResponse{
		Success: true,
		Message: "You're on the waitlist! 🎉",
	}, nil
}

func main() {
	log.Println("Starting the waitlist server...")

	// Load all environmental variables
	err := config.LoadSharedConfig()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatalf("DATABASE_URL environment variable is required")
	}

	// Load the TLS configuration values
	tlsConfig, err := config.LoadServerTLSFromEnv("WAITLIST_CRT", "WAITLIST_KEY")
	if err != nil {
		log.Fatal("Error loading TLS config for waitlist service")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS waitlist (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email VARCHAR(255) UNIQUE NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		log.Fatalf("Failed to create tables: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS email_idx ON waitlist(email)
	`)
	if err != nil {
		log.Fatalf("Failed to create email index: %v", err)
	}

	grpcAddress := os.Getenv("WAITLIST_PORT")
	if grpcAddress == "" {
		log.Fatal("WAITLIST_PORT environment variable is required")
	}

	listener, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	log.Println("Creating the waitlist server...")

	opts := []grpc.ServerOption{
		grpc.Creds(credentials.NewTLS(tlsConfig)),
	}
	grpcServer := grpc.NewServer(opts...)
	pb.RegisterWaitlistServiceServer(grpcServer, &WaitlistServer{db: db})

	log.Printf("Waitlist Service listening on %v\n", listener.Addr())
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	} else {
		log.Println("Waitlist server started successfully")
	}
}
