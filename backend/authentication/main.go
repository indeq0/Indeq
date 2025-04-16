package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	pb "github.com/cc-0000/indeq/common/api"
	"github.com/cc-0000/indeq/common/config"
	"github.com/golang-jwt/jwt/v5"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/argon2"
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
	integrationClient pb.IntegrationServiceClient
	jwtSecret         []byte // secret for creating jwts
	argonParams       *params
	MinPasswordLength int
	MaxPasswordLength int
	MaxEmailLength    int
}

// func()
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

// func(password string)
//   - makes sure the password is within the configured min/max lengths
//   - assumes: parameters are loaded into memory already (via loadPasswordSettings() or otherwise)
func (s *authServer) validatePassword(password string) error {
	if len(password) < s.MinPasswordLength {
		return fmt.Errorf("password must be at least %d characters", s.MinPasswordLength)
	}
	if len(password) > s.MaxPasswordLength {
		return fmt.Errorf("password must not exceed %d characters", s.MaxPasswordLength)
	}
	return nil
}

// func(email string)
//   - checks to make sure the email is: within configured min/max lengths & formatted via RFC 5322
func (s *authServer) validateEmail(email string) error {
	if len(email) > s.MaxEmailLength {
		return fmt.Errorf("email must not exceed %d characters", s.MaxEmailLength)
	}
	if len(email) <= 0 {
		return fmt.Errorf("email must not be blank")
	}

	// Check email format
	_, err := mail.ParseAddress(strings.TrimSpace(email))
	if err != nil {
		return fmt.Errorf("invalid email format: %w", err)
	}

	return nil
}

// TODO: implement name validation here
func (s *authServer) validateName(name string) error {
	return nil
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

// func(password string, hashed password to compare to)
//   - hashes the incoming password using the same settings as the encoded hash and compares them
//   - returns (whether or not the password is right, any error)
func comparePasswordAndEncodedHash(password string, encodedHash string) (bool, error) {
	// Unencode the configuration variables from the password hash and salt
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false, fmt.Errorf("invalid hash format")
	}

	// Get the version
	var version int
	_, err := fmt.Sscanf(parts[2], "v=%d", &version)
	if err != nil {
		return false, err
	}

	// Get the memory constraint, number of iterations, and amnt of parallelism
	var memory uint32
	var iterations uint32
	var parallelism uint8
	_, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism)
	if err != nil {
		return false, err
	}

	// Get the salt
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}

	// Get the hash
	decodedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}

	// Compute the hash of the incoming password
	computedHash := argon2.IDKey(
		[]byte(password),
		salt,
		iterations,
		memory,
		parallelism,
		uint32(len(decodedHash)),
	)

	// Constant-time comparison
	return subtle.ConstantTimeCompare(computedHash, decodedHash) == 1, nil
}

// rpc(context, login request)
//   - takes a username and password and tries to find a matching entry in our user database
//   - fails if rate limited or (user, password) is not a match
//   - returns a new JWT and user id on success
func (s *authServer) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	// Rate limit
	if exceeded, err := s.checkRateLimit(ctx, req.Email); err != nil {
		return nil, err
	} else if exceeded {
		return &pb.LoginResponse{Error: "too many attempts, please try again later"}, nil
	}

	// get the id and encoded password hash matching user email
	var id string
	var encodedHash string
	err := s.db.QueryRowContext(ctx,
		"SELECT id, password_hash FROM users WHERE email = $1",
		strings.ToLower(req.Email),
	).Scan(&id, &encodedHash)

	if err == sql.ErrNoRows {
		// Even though thhe user doesn't exist we want to fake a comparison
		dummyEncodedHash := fmt.Sprintf(
			"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
			argon2.Version,
			s.argonParams.memory,
			s.argonParams.iterations,
			s.argonParams.parallelism,
			"AAAAAAAAAAAAAAAA",
			"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		)
		comparePasswordAndEncodedHash(req.Password, dummyEncodedHash)
		return &pb.LoginResponse{Error: "invalid credentials"}, nil
	}
	if err != nil {
		return nil, err
	}

	match, err := comparePasswordAndEncodedHash(req.Password, encodedHash)
	if err != nil {
		return &pb.LoginResponse{Error: "invalid credentials"}, nil
	}
	if !match {
		// Increment failed attempts counter
		s.incrementFailedAttempts(ctx, req.Email)
		return &pb.LoginResponse{Error: "invalid credentials"}, nil
	}
	s.resetFailedAttempts(ctx, req.Email)

	currentTime := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": id,
		"exp": currentTime.Add(24 * time.Hour).Unix(), // current 1 day expiration
		"iat": currentTime.Unix(),
		"nbf": currentTime.Unix(),
	})

	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return nil, err
	}

	return &pb.LoginResponse{Token: tokenString, UserId: id}, nil
}

// TODO: implement email-sending validation with redis
// rpc(context, register request)
//   - takes in a email, name and password and registers the user in our database
//   - email, password must pass validation
//   - creates corresponding Vector, and Desktop datastores for the user
func (s *authServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	// Make sure email is good
	if err := s.validateEmail(req.Email); err != nil {
		return &pb.RegisterResponse{
			Success: false,
			Error:   fmt.Sprintf("invalid email: %v", err),
		}, err
	}

	// Make sure name is good
	if err := s.validateName(req.Name); err != nil {
		return &pb.RegisterResponse{
			Success: false,
			Error:   fmt.Sprintf("invalid name: %v", err),
		}, err
	}

	// Make sure password is good
	if err := s.validatePassword(req.Password); err != nil {
		return &pb.RegisterResponse{
			Success: false,
			Error:   fmt.Sprintf("invalid password: %v", err),
		}, err
	}

	// Generate a random salt
	salt := make([]byte, s.argonParams.saltLength)
	if _, err := rand.Read(salt); err != nil {
		return &pb.RegisterResponse{
			Success: false,
			Error:   fmt.Sprintf("couldn't make a salt: %v", err),
		}, err
	}

	// Generate a password hash
	hash := argon2.IDKey(
		[]byte(req.Password),
		salt,
		s.argonParams.iterations,
		s.argonParams.memory,
		s.argonParams.parallelism,
		s.argonParams.keyLength,
	)

	// Keep encryption details alongside the hash
	encodedHash := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		s.argonParams.memory,
		s.argonParams.iterations,
		s.argonParams.parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)

	// Store in the database
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return &pb.RegisterResponse{
			Success: false,
			Error:   err.Error(),
		}, fmt.Errorf("failed to begin transaction: %v", err)
	}
	var userId string
	var googleId string
	var passwordHash sql.NullString
	err = tx.QueryRowContext(
		ctx,
		"SELECT id, google_id, password_hash FROM users WHERE email = $1",
		strings.ToLower(req.Email), // Normalize email
	).Scan(&userId, &googleId, &passwordHash)

	if err != sql.ErrNoRows {

		// Google ID exists without a password hash
		if err == nil && googleId != "" && (passwordHash.String == "") {
			err = tx.QueryRowContext(
				ctx,
				"UPDATE users SET password_hash = $1 WHERE email = $2 RETURNING id",
				encodedHash,
				strings.ToLower(req.Email),
			).Scan()
			if err := tx.Commit(); err != nil {
				return nil, fmt.Errorf("failed to commit transaction: %v", err)
			}
			return &pb.RegisterResponse{Success: true}, nil
		}
		return &pb.RegisterResponse{Success: false, Error: "email already exists"}, nil
	}

	defer tx.Rollback()

	err = tx.QueryRowContext(
		ctx,
		"INSERT INTO users (email, password_hash, name) VALUES ($1, $2, $3) RETURNING id",
		strings.ToLower(req.Email), // Normalize email
		encodedHash,
		req.Name,
	).Scan(&userId)

	// Try to create a corresponding entry in the desktop tracking collection
	dRes, err := s.desktopClient.SetupUserStats(ctx, &pb.SetupUserStatsRequest{
		UserId: userId,
	})
	if err != nil || !dRes.Success {
		return &pb.RegisterResponse{
			Success: false,
			Error:   "failed to setup user datastores",
		}, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &pb.RegisterResponse{Success: true}, nil
}

// rpc(context, verify request)
//   - takes in a jwt and checks to make sure it's valid
//   - returns the user id of the jwt on success
func (s *authServer) Verify(ctx context.Context, req *pb.VerifyRequest) (*pb.VerifyResponse, error) {
	// parse out the token
	token, err := jwt.Parse(req.Token, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return s.jwtSecret, nil
	})

	// check if token was able to be parsed
	if err != nil {
		log.Printf("Failed to parse token: %v", err)
		return &pb.VerifyResponse{Valid: false, Error: "invalid token"}, nil
	}

	// verify validity of token
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return &pb.VerifyResponse{
			Valid:  true,
			UserId: claims["sub"].(string),
		}, nil
	}

	return &pb.VerifyResponse{Valid: false, Error: "invalid token"}, nil
}

// rpc(context, sign csr request)
//   - takes a base64 csr, signs it, and returns a base64 signed certificate
//   - takes an adjacent login request to make sure the user is authenticate to get this certificate
func (s *authServer) SignCSR(ctx context.Context, req *pb.SignCSRRequest) (*pb.SignCSRResponse, error) {
	// try to authenticate the user first
	loginRes, err := s.Login(ctx, req.LoginRequest)
	if err != nil {
		return nil, fmt.Errorf("user is not authenticate to make csr request: %v", err)
	}
	userId := loginRes.GetUserId()

	// Decode the base64 CSR
	csrBytes, err := base64.StdEncoding.DecodeString(req.CsrBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode CSR: %v", err)
	}

	// if it's in PEM format we want to extract it in DER format
	block, _ := pem.Decode(csrBytes)
	if block != nil && block.Type == "CERTIFICATE REQUEST" {
		csrBytes = block.Bytes
	}

	// Parse the CSR
	csr, err := x509.ParseCertificateRequest(csrBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSR: %v", err)
	}

	// Verify the CSR signature
	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("invalid CSR signature: %v", err)
	}

	// Verify the user ID in the CSR subject matches the requested user ID
	uidFound := false
	for _, name := range csr.Subject.Names {
		if name.Type.String() == "0.9.2342.19200300.100.1.1" || name.Type.String() == "2.5.4.3" { // OID for UID and CN
			if name.Value.(string) == userId {
				uidFound = true
				break
			}
		}
	}

	if !uidFound {
		return nil, fmt.Errorf("user ID in CSR does not match authenticated user")
	}

	// Get CA certificate and key from environment variables
	caCertPEM := os.Getenv("CA_CRT")
	caKeyPEM := os.Getenv("CA_KEY")

	if caCertPEM == "" || caKeyPEM == "" {
		return nil, fmt.Errorf("CA certificate or key not found in environment variables")
	}

	// Decode base64 if needed
	var caCertData, caKeyData []byte
	var decodeErr error

	// Try to decode as base64 first
	caCertData, decodeErr = base64.StdEncoding.DecodeString(caCertPEM)
	if decodeErr != nil {
		return nil, fmt.Errorf("CA certificate not in base64 format")
	}

	caKeyData, decodeErr = base64.StdEncoding.DecodeString(caKeyPEM)
	if decodeErr != nil {
		return nil, fmt.Errorf("CA key not in base64 format")
	}

	// Parse CA certificate - assuming PEM format
	block, _ = pem.Decode(caCertData)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("failed to decode CA certificate PEM")
	}

	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA certificate: %v", err)
	}

	// Parse CA private key - assuming PEM format
	block, _ = pem.Decode(caKeyData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode CA private key PEM")
	}

	var caKey any

	// Try parsing as PKCS8 first (which is what openssl genpkey produces)
	caKey, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// If PKCS8 fails, try PKCS1
		caKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return &pb.SignCSRResponse{
				CertificateBase64: "",
			}, fmt.Errorf("failed to parse CA private key: %v", err)
		}
	}

	// Ensure we have an RSA private key
	rsaKey, ok := caKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("CA private key is not an RSA key")
	}

	// generate a random serial number for the certificate
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number")
	}

	// Prepare certificate template
	now := time.Now()
	template := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               csr.Subject,
		NotBefore:             now,
		NotAfter:              now.Add(365 * 24 * time.Hour), // TODO: implement certificate rotation
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Create the certificate
	certDERBytes, err := x509.CreateCertificate(
		rand.Reader,
		&template,
		caCert,
		csr.PublicKey,
		rsaKey,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %v", err)
	}

	// Convert DER to PEM format
	certPEM := &bytes.Buffer{}
	err = pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDERBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode certificate to PEM: %v", err)
	}

	// Encode the PEM certificate to base64
	certBase64 := base64.StdEncoding.EncodeToString(certPEM.Bytes())

	return &pb.SignCSRResponse{
		CertificateBase64: certBase64,
	}, nil
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
				"INSERT INTO users (email, name, google_id) VALUES ($1, $2, $3) RETURNING id",
				strings.ToLower(req.Email),
				req.Name,
				req.GoogleId,
			).Scan(&userId)
		} else {
			// Create with just name and google_id
			err = tx.QueryRowContext(ctx,
				"INSERT INTO users (name, google_id) VALUES ($1, $2) RETURNING id",
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

	<-sigChan // TODO: implement worker groups
	log.Print("gracefully shutting down...")
}
