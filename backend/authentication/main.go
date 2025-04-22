package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	pb "github.com/cc-0000/indeq/common/api"
	"github.com/cc-0000/indeq/common/config"
	"github.com/cc-0000/indeq/common/redis"
	"github.com/golang-jwt/jwt/v5"
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
	db                 *sql.DB // password database
	desktopConn        *grpc.ClientConn
	desktopClient      pb.DesktopServiceClient
	integrationConn    *grpc.ClientConn
	integrationService pb.IntegrationServiceClient
	queryConn          *grpc.ClientConn
	queryService       pb.QueryServiceClient
	integrationClient  pb.IntegrationServiceClient
	jwtSecret          []byte // secret for creating jwts
	argonParams        *params
	MinPasswordLength  int
	MaxPasswordLength  int
	MaxEmailLength     int
	redisClient        *redis.RedisClient
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
            email VARCHAR(255) UNIQUE,
            password_hash TEXT,
            name VARCHAR(255) NOT NULL,
            google_id VARCHAR(255) UNIQUE,
			alias VARCHAR(255) NOT NULL,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            CONSTRAINT password_required_if_no_google CHECK (
                (google_id IS NOT NULL) OR
                (google_id IS NULL AND password_hash IS NOT NULL)
            ),
			CONSTRAINT email_required_if_google CHECK (
                (google_id IS NOT NULL) OR
                (google_id IS NULL AND email IS NOT NULL)
            )
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

// rpc(context, create or update google user request)
//   - takes in a google id, name, and optional email
//   - if email is provided, looks for matching user and updates google id/name if needed
//   - if email is not provided, looks for user with matching google_id, creates new if none found
//   - sets userstats on desktop
//   - returns the user id, jwt on success
func (s *authServer) CreateOrUpdateGoogleUser(ctx context.Context, req *pb.CreateOrUpdateGoogleUserRequest) (*pb.CreateOrUpdateGoogleUserResponse, error) {

	// Start a transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// First, check if user exists by email or google_id
	var userId string
	var existingEmail sql.NullString
	var existingName sql.NullString
	var existingGoogleId sql.NullString
	var userExists bool
	var userCreated bool = false

	// Try to find user by email if provided and not empty
	if req.Email != "" {
		err = tx.QueryRowContext(ctx,
			"SELECT id, email, name, google_id FROM users WHERE email = $1",
			strings.ToLower(req.Email),
		).Scan(&userId, &existingEmail, &existingName, &existingGoogleId)

		if err == nil {
			userExists = true
		} else if err != sql.ErrNoRows {
			return nil, fmt.Errorf("database error when checking email: %v", err)
		}
	}

	// If user not found by email or email not provided, try to find by google_id
	if !userExists {
		err = tx.QueryRowContext(ctx,
			"SELECT id, email, name, google_id FROM users WHERE google_id = $1",
			req.GoogleId,
		).Scan(&userId, &existingEmail, &existingName, &existingGoogleId)

		if err == nil {
			userExists = true
		} else if err != sql.ErrNoRows {
			return nil, fmt.Errorf("database error when checking google_id: %v", err)
		}
	}

	if userExists {
		// User exists, determine what fields to update
		updates := []string{}
		args := []interface{}{}
		argCount := 1

		// Update google_id if it's empty and we have a new one
		if !existingGoogleId.Valid || existingGoogleId.String == "" {
			updates = append(updates, fmt.Sprintf("google_id = $%d", argCount))
			args = append(args, req.GoogleId)
			argCount++
		}

		// Update name if it's empty
		if !existingName.Valid || existingName.String == "" {
			updates = append(updates, fmt.Sprintf("name = $%d", argCount))
			args = append(args, req.Name)
			argCount++
		}

		// Update email if it's empty and we have a new one
		if req.Email != "" && (!existingEmail.Valid || existingEmail.String == "") {
			updates = append(updates, fmt.Sprintf("email = $%d", argCount))
			args = append(args, strings.ToLower(req.Email))
			argCount++
		}

		// If we have fields to update, perform the update
		if len(updates) > 0 {
			args = append(args, userId)
			query := fmt.Sprintf("UPDATE users SET %s WHERE id = $%d RETURNING id",
				strings.Join(updates, ", "),
				argCount)

			err = tx.QueryRowContext(ctx, query, args...).Scan(&userId)
			if err != nil {
				return nil, fmt.Errorf("failed to update user: %v", err)
			}
		}
	} else {
		// User doesn't exist, create a new one
		if req.Email != "" {
			// Create with email, name, and google_id
			err = tx.QueryRowContext(ctx,
				"INSERT INTO users (email, name, google_id, alias) VALUES ($1, $2, $3, $2) RETURNING id",
				strings.ToLower(req.Email),
				req.Name,
				req.GoogleId,
			).Scan(&userId)
		} else {
			// Create with just name and google_id
			err = tx.QueryRowContext(ctx,
				"INSERT INTO users (name, google_id, alias) VALUES ($1, $2, $1) RETURNING id",
				req.Name,
				req.GoogleId,
			).Scan(&userId)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to create user: %v", err)
		}

		// Try to create a corresponding entry in the desktop tracking collection
		dRes, err := s.desktopClient.SetupUserStats(ctx, &pb.SetupUserStatsRequest{
			UserId: userId,
		})
		if err != nil || !dRes.Success {
			log.Printf("Failed to set up userstore")
		}
		userCreated = true

	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		userCreated = false
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	// Create JWT token
	currentTime := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userId,
		"exp": currentTime.Add(24 * time.Hour).Unix(), // 1 day expiration
		"iat": currentTime.Unix(),
		"nbf": currentTime.Unix(),
	})

	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %v", err)
	}

	return &pb.CreateOrUpdateGoogleUserResponse{
		UserId:      userId,
		Token:       tokenString,
		UserCreated: userCreated,
	}, nil
}

func (s *authServer) SSOLogin(ctx context.Context, req *pb.SSOConnectRequest) (*pb.SSOConnectResponse, error) {
	// Check if the environment variables are properly set
	clientID := os.Getenv("GOOGLE_SSO_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_SSO_CLIENT_SECRET")
	redirectURI := os.Getenv("GOOGLE_SSO_REDIRECT_URI")

	if clientID == "" {
		return nil, fmt.Errorf("server configuration error")
	}

	if clientSecret == "" {
		return nil, fmt.Errorf("server configuration error")
	}

	if redirectURI == "" {
		return nil, fmt.Errorf("server configuration error")
	}

	// Since SSOConnectRequest doesn't have a State field, we'll use a default state
	ssoState := "sso:" + req.State

	validateRes, err := s.integrationClient.ValidateOAuthState(ctx, &pb.ValidateOAuthStateRequest{
		State: ssoState,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to validate SSO state: %v", err)
	}

	if !validateRes.Success {
		return nil, fmt.Errorf(validateRes.ErrorDetails)
	}

	// Exchange the authorization code for tokens
	tokenURL := "https://oauth2.googleapis.com/token"
	data := url.Values{}
	data.Set("code", req.AuthCode)
	data.Set("client_id", os.Getenv("GOOGLE_SSO_CLIENT_ID"))
	data.Set("client_secret", os.Getenv("GOOGLE_SSO_CLIENT_SECRET"))
	data.Set("redirect_uri", os.Getenv("GOOGLE_SSO_REDIRECT_URI"))
	data.Set("grant_type", "authorization_code")

	// Create the HTTP request
	httpReq, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token exchange request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Send the request
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for tokens: %v", err)
	}
	defer resp.Body.Close()

	// Create a new reader with the body bytes for subsequent json.Decode
	var bodyBytes []byte
	bodyBytes, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}

	// Decode the token response
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %v", err)
	}

	// Check if we got an access token
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("failed to get access token")
	}

	// Use the access token to get the user's email from Google
	userInfoURL := "https://www.googleapis.com/oauth2/v2/userinfo"
	httpReq, err = http.NewRequest("GET", userInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create user info request: %v", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	userInfoResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %v", err)
	}
	defer userInfoResp.Body.Close()

	// Read and log the response body
	bodyBytes, err = io.ReadAll(userInfoResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	// Check if the response is successful
	if userInfoResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user info")
	}

	// Create a new reader with the body bytes for subsequent decoding
	userInfoResp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var userInfo struct {
		ID            string `json:"id"`
		Email         string `json:"email"`
		VerifiedEmail bool   `json:"verified_email"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}

	if err := json.NewDecoder(userInfoResp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %v", err)
	}

	// Check if we got the required user info
	if userInfo.Email == "" {
		return nil, fmt.Errorf("failed to get user email")
	}

	// Call authentication service to create/update Google user
	userResponse, err := s.CreateOrUpdateGoogleUser(ctx, &pb.CreateOrUpdateGoogleUserRequest{
		GoogleId: userInfo.ID,
		Name:     userInfo.Name,
		Email:    userInfo.Email,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create/update user: %v", err)
	}

	respBody := &pb.SSOConnectResponse{
		Success:      true,
		Message:      "Successfully authenticated with Google",
		ErrorDetails: "",
		Token:        userResponse.Token,
		UserId:       userResponse.UserId,
		UserCreated:  userResponse.UserCreated,
	}

	return respBody, nil
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

	// Connect to the integration service
	integrationAddy, ok := os.LookupEnv("INTEGRATION_ADDRESS")
	if !ok {
		log.Fatal("failed to retrieve integration address for connection")
	}
	integrationConn, err := grpc.NewClient(
		integrationAddy,
		grpc.WithTransportCredentials(credentials.NewTLS(clientTlsConfig)),
	)
	if err != nil {
		log.Fatalf("Failed to establish connection with integration-service: %v", err)
	}
	defer integrationConn.Close()
	server.integrationClient = pb.NewIntegrationServiceClient(integrationConn)

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
