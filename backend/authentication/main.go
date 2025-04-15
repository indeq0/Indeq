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
	"log"
	"math/big"
	"net"
	"net/mail"
	"net/smtp"
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
	"github.com/google/uuid"
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
	jwtSecret         []byte // secret for creating jwts
	argonParams       *params
	MinPasswordLength int
	MaxPasswordLength int
	MaxEmailLength    int
	redisClient       *redis.RedisClient
}

type RegistrationPayload struct {
	Email          string `json:"email"`
	HashedPassword string `json:"hashed_password"`
	Name           string `json:"name"`
	OTP            string `json:"otp"`
}

type ForgotPayload struct {
	Email string `json:"email"`
	OTP   string `json:"otp"`
}

type Config struct {
	KeyPrefix string
	Limit     int
	Duration  time.Duration
}

func generateOTP() (string, error) {
	const digits = "0123456789"
	const length = 6
	otp := make([]byte, 0, length)
	// Use 250 as the maximum to avoid modulo bias
	max := byte(250)
	// buffer to read random bytes in batches
	buf := make([]byte, 16)
	for len(otp) < length {
		_, err := rand.Read(buf)
		if err != nil {
			return "", fmt.Errorf("failed to generate OTP: %w", err)
		}
		for _, b := range buf {
			if b > max {
				continue
			}
			otp = append(otp, digits[b%10])
			if len(otp) == length {
				break
			}
		}
	}

	return string(otp), nil
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
		return &pb.LoginResponse{Error: "Invalid credentials"}, nil
	}
	if err != nil {
		return nil, err
	}

	match, err := comparePasswordAndEncodedHash(req.Password, encodedHash)
	if err != nil {
		return &pb.LoginResponse{Error: "Invalid credentials"}, nil
	}
	if !match {
		// Increment failed attempts counter
		s.incrementFailedAttempts(ctx, req.Email)
		return &pb.LoginResponse{Error: "Invalid credentials"}, nil
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

// func(to string, subject string, body string)
//   - sends an email to the given address with the given subject and body
//   - returns an error if the email fails to send
func (s *authServer) sendEmail(to string, subject string, body string) error {
	from := os.Getenv("SMTP_FROM")
	user := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASS")
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")

	// check if all the env variables are set
	if from == "" || user == "" || pass == "" || host == "" || port == "" {
		return fmt.Errorf("SMTP configuration is incomplete")
	}

	// set up the address
	addr := fmt.Sprintf("%s:%s", host, port)

	// set up the TLS configuration
	tlsConfig := &tls.Config{
		ServerName: host,
	}

	// dial the TLS connection
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to dial TLS connection: %w", err)
	}
	defer conn.Close()

	// create the SMTP client
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	// set up the authentication
	auth := smtp.PlainAuth("", user, pass, host)
	if ok, _ := client.Extension("AUTH"); ok {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("failed to authenticate: %w", err)
		}
	}

	// set the sender
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	// set the recipient
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("failed to set recipient: %w", err)
	}

	// start the data command
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to start data command: %w", err)
	}

	// set up the message
	msg := []byte("From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=\"utf-8\"\r\n" +
		"\r\n" +
		body + "\r\n")

	// write the message
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	// close the data writer
	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	// quit the client
	if err := client.Quit(); err != nil {
		return fmt.Errorf("failed to quit SMTP client: %w", err)
	}

	return nil
}

// rpc(context, register request)
//   - takes in a email, name and password and registers the user in our database
//   - email, password must pass validation
//   - creates corresponding Vector, and Desktop datastores for the user
func (s *authServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	// Make sure email is good
	if err := s.validateEmail(req.Email); err != nil {
		return &pb.RegisterResponse{
			Success: false,
			Error:   fmt.Sprintf("Invalid email: %v", err),
		}, err
	}

	// Make sure name is good
	if err := s.validateName(req.Name); err != nil {
		return &pb.RegisterResponse{
			Success: false,
			Error:   fmt.Sprintf("Invalid name: %v", err),
		}, err
	}

	// Make sure password is good
	if err := s.validatePassword(req.Password); err != nil {
		return &pb.RegisterResponse{
			Success: false,
			Error:   fmt.Sprintf("Invalid password: %v", err),
		}, err
	}

	// Check if user already exists
	var existingUser string
	err := s.db.QueryRowContext(ctx, "SELECT id FROM users WHERE email = $1", req.Email).Scan(&existingUser)
	if err != nil && err != sql.ErrNoRows {
		return &pb.RegisterResponse{
			Success: false,
			Error:   "Something went wrong. Please try again later.",
		}, nil
	}

	if existingUser != "" {
		return &pb.RegisterResponse{
			Success: false,
			Error:   "Email already exists!",
		}, nil
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

	// Generate a random OTP
	otp, err := generateOTP()
	if err != nil {
		// if the OTP generation fails, return an error
		return &pb.RegisterResponse{
			Success: false,
			Error:   "Something went wrong. Please try again later.",
		}, err
	}

	// Generate a random token
	token := uuid.NewString()

	// Store the token in Redis
	redisKey := fmt.Sprintf("reg:%s", token)

	payload := RegistrationPayload{
		Email:          req.Email,
		HashedPassword: encodedHash,
		Name:           req.Name,
		OTP:            otp,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		// if the payload marshalling fails, return an error
		return &pb.RegisterResponse{
			Success: false,
			Error:   "Something went wrong. Please try again later.",
		}, err
	}

	err = s.redisClient.Set(ctx, redisKey, payloadBytes, 5*time.Minute)
	if err != nil {
		// if the redis set fails, return an error
		return &pb.RegisterResponse{
			Success: false,
			Error:   "Something went wrong. Please try again later.",
		}, err
	}

	// Compose the email body for verification.
	// do not change format
	emailBody := fmt.Sprintf(`Welcome to Indeq!

To verify your account, enter the following 6-digit code:

Your verification code: %s

This code will expire in 5 minutes. If you did not request this verification code, you can safely ignore this email.

Thank you,
The Indeq Team
`, otp)

	emailSubject := "Indeq - Verify Your Account"
	err = s.sendEmail(req.Email, emailSubject, emailBody)
	if err != nil {
		// if the email sending fails, return an error
		return &pb.RegisterResponse{
			Success: false,
			Error:   "Something went wrong. Please try again later.",
		}, err
	}

	return &pb.RegisterResponse{
		Success: true,
		Token:   token,
	}, nil
}

// rpc(context, resend otp request)
//   - takes in a token and a type and resends the otp
//   - returns a success boolean and a user id on success
//   - returns an error on failure
//   - if the type is register, it will resend the verification email
//   - if the type is forgot, it will resend the password reset email
func (s *authServer) ResendOTP(ctx context.Context, req *pb.ResendOTPRequest) (*pb.ResendOTPResponse, error) {
	if req.Type != "register" && req.Type != "forgot" {
		return &pb.ResendOTPResponse{
			Success: false,
			Error:   "Invalid verification type",
		}, nil
	}

	var redisKey string
	if req.Type == "register" {
		redisKey = fmt.Sprintf("reg:%s", req.Token)
	} else if req.Type == "forgot" {
		redisKey = fmt.Sprintf("forgot:%s", req.Token)
	}

	data, err := s.redisClient.Get(ctx, redisKey)
	if err != nil {
		// if the redis get fails, return an error
		return &pb.ResendOTPResponse{
			Success: false,
			Error:   "Something went wrong. Please try again later.",
		}, err
	}

	if data == "" {
		// if the data is empty, return an error
		return &pb.ResendOTPResponse{
			Success: false,
			Error:   "Something went wrong. Please try again later.",
		}, nil
	}
	newOTP, err := generateOTP()
	if err != nil {
		// if the OTP generation fails, return an error
		return &pb.ResendOTPResponse{
			Success: false,
			Error:   "Something went wrong. Please try again later.",
		}, err
	}

	if req.Type == "register" {
		var payload RegistrationPayload
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			// if the payload unmarshalling fails, return an error
			return &pb.ResendOTPResponse{
				Success: false,
				Error:   "Invalid data",
			}, nil
		}

		payload.OTP = newOTP

		updatedPayload, err := json.Marshal(payload)
		if err != nil {
			// if the payload marshalling fails, return an error
			return &pb.ResendOTPResponse{
				Success: false,
				Error:   "Something went wrong. Please try again later.",
			}, err
		}

		// store the updated payload in redis and reset timer
		err = s.redisClient.Set(ctx, redisKey, updatedPayload, 5*time.Minute)
		if err != nil {
			// if the redis set fails, return an error
			return &pb.ResendOTPResponse{
				Success: false,
				Error:   "Something went wrong. Please try again later.",
			}, err
		}

		// Compose the email body for verification.
		// do not change format
		emailBody := fmt.Sprintf(`Welcome to Indeq!

To verify your account, enter the following 6-digit code:

Your verification code: %s

This code will expire in 5 minutes. If you did not request this verification code, please ignore this email.

Thank you,
The Indeq Team
`, newOTP)
		emailSubject := "Indeq - Verify Your Account"

		if err := s.sendEmail(payload.Email, emailSubject, emailBody); err != nil {
			// if the email sending fails, return an error
			return &pb.ResendOTPResponse{
				Success: false,
				Error:   "Failed to send verification email",
			}, err
		}
	} else if req.Type == "forgot" {
		var payload ForgotPayload
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			// if the payload unmarshalling fails, return an error
			return &pb.ResendOTPResponse{
				Success: false,
				Error:   "Something went wrong. Please try again later.",
			}, nil
		}

		payload.OTP = newOTP

		updatedPayload, err := json.Marshal(payload)
		if err != nil {
			// if the payload marshalling fails, return an error
			return &pb.ResendOTPResponse{
				Success: false,
				Error:   "Something went wrong. Please try again later.",
			}, err
		}

		// store the updated payload in redis and reset timer
		err = s.redisClient.Set(ctx, redisKey, updatedPayload, 5*time.Minute)
		if err != nil {
			// if the redis set fails, return an error
			return &pb.ResendOTPResponse{
				Success: false,
				Error:   "Something went wrong. Please try again later.",
			}, err
		}

		// Compose the email body for password reset.
		// do not change format
		emailBody := fmt.Sprintf(`Hi there,

You requested a password reset.

To reset your password, enter the following 6-digit code:

Your verification code: %s

This code will expire in 5 minutes. If you did not request a password reset, please ignore this email.

Thank you,
The Indeq Team
`, newOTP)
		emailSubject := "Indeq - Reset Your Password"

		// Send the email.
		if err := s.sendEmail(payload.Email, emailSubject, emailBody); err != nil {
			// if the email sending fails, return an error
			return &pb.ResendOTPResponse{
				Success: false,
				Error:   "Failed to send verification email",
			}, err
		}
	}

	return &pb.ResendOTPResponse{
		Success: true,
	}, nil
}

// rpc(context, verify otp request)
//   - takes in a token and a code and verifies the code
//   - returns a success boolean and a user id on success
//   - returns an error on failure
//   - if the type is register, it will store the user in the database
//   - if the type is forgot, it will update the user's password
func (s *authServer) VerifyOTP(ctx context.Context, req *pb.VerifyOTPRequest) (*pb.VerifyOTPResponse, error) {
	if req.Code == "" {
		// if the code is empty, return an error
		return &pb.VerifyOTPResponse{
			Success: false,
			Error:   "Code is required",
		}, nil
	}

	if req.Type != "register" && req.Type != "forgot" {
		// if the type is invalid, return an error
		return &pb.VerifyOTPResponse{
			Success: false,
			Error:   "Invalid verification type",
		}, nil
	}

	if req.Token == "" {
		// if the token is empty, return an error
		return &pb.VerifyOTPResponse{
			Success: false,
			Error:   "No token found",
		}, nil
	}

	var redisKey string
	if req.Type == "register" {
		redisKey = fmt.Sprintf("reg:%s", req.Token)
	} else if req.Type == "forgot" {
		redisKey = fmt.Sprintf("forgot:%s", req.Token)
	}
	data, err := s.redisClient.Get(ctx, redisKey)
	if err != nil {
		// if the redis get fails, return an error
		return &pb.VerifyOTPResponse{
			Success: false,
			Error:   "Something went wrong. Please try again later.",
		}, nil
	}

	if req.Type == "register" {
		var payload RegistrationPayload
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			// if the payload unmarshalling fails, return an error
			return &pb.VerifyOTPResponse{
				Success: false,
				Error:   "Something went wrong. Please try again later.",
			}, nil
		}

		if payload.OTP != req.Code {
			// if the code is invalid, return an error
			return &pb.VerifyOTPResponse{
				Success: false,
				Error:   "Invalid code!",
			}, nil
		}

		// Store in the database
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			// if the transaction fails, return an error
			return &pb.VerifyOTPResponse{
				Success: false,
				Error:   "Something went wrong. Please try again later.",
			}, fmt.Errorf("failed to begin transaction: %v", err)
		}
		defer tx.Rollback()

		var userId string
		err = tx.QueryRowContext(
			ctx,
			"INSERT INTO users (email, password_hash, name) VALUES ($1, $2, $3) RETURNING id",
			strings.ToLower(payload.Email), // Normalize email
			payload.HashedPassword,
			payload.Name,
		).Scan(&userId)

		if err != nil {
			// if the query fails, return an error
			return &pb.VerifyOTPResponse{
				Success: false,
				Error:   "email already exists",
			}, err
		}

		// Try to create a corresponding entry in the desktop tracking collection
		dRes, err := s.desktopClient.SetupUserStats(ctx, &pb.SetupUserStatsRequest{
			UserId: userId,
		})
		if err != nil || !dRes.Success {
			// if the desktop client call fails, return an error
			return &pb.VerifyOTPResponse{
				Success: false,
				Error:   "failed to setup user datastores",
			}, err
		}

		if err := tx.Commit(); err != nil {
			// if the transaction fails, return an error
			return &pb.VerifyOTPResponse{
				Success: false,
				Error:   "failed to commit transaction",
			}, err
		}

		currentTime := time.Now()
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub": userId,
			"exp": currentTime.Add(24 * time.Hour).Unix(), // current 1 day expiration
			"iat": currentTime.Unix(),
			"nbf": currentTime.Unix(),
		})

		tokenString, err := token.SignedString(s.jwtSecret)
		if err != nil {
			// if the token signing fails, return an error
			return &pb.VerifyOTPResponse{
				Success: false,
				Error:   "failed to create token",
			}, err
		}

		// Delete the token from redis
		s.redisClient.Del(ctx, redisKey)
		return &pb.VerifyOTPResponse{
			Success: true,
			Token:   tokenString,
			UserId:  userId,
		}, nil
	} else if req.Type == "forgot" {
		var payload ForgotPayload
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			// if the payload unmarshalling fails, return an error
			return &pb.VerifyOTPResponse{
				Success: false,
				Error:   "Something went wrong. Please try again later.",
			}, nil
		}

		if payload.OTP != req.Code {
			// if the code is invalid, return an error
			return &pb.VerifyOTPResponse{
				Success: false,
				Error:   "Invalid code!",
			}, nil
		}

		return &pb.VerifyOTPResponse{
			Success: true,
		}, nil
	}

	return &pb.VerifyOTPResponse{
		Success: false,
		Error:   "Invalid verification type",
	}, nil
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

// rpc(context, forgot password request)
//   - takes in an email and sends a reset password email to the user
//   - returns a token on success
func (s *authServer) ForgotPassword(ctx context.Context, req *pb.ForgotPasswordRequest) (*pb.ForgotPasswordResponse, error) {
	if req.Email == "" {
		return &pb.ForgotPasswordResponse{
			Success: false,
			Error:   "Email is required",
		}, nil
	}

	// normalize the email
	req.Email = strings.ToLower(req.Email)

	// generate a random OTP
	otp, err := generateOTP()
	if err != nil {
		// if the OTP generation fails, return an error
		return &pb.ForgotPasswordResponse{
			Success: false,
			Error:   "Something went wrong. Please try again later.",
		}, err
	}

	token := uuid.New().String()
	redisKey := fmt.Sprintf("forgot:%s", token)

	// create a payload for the email
	payload := ForgotPayload{
		Email: req.Email,
		OTP:   otp,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		// if the payload marshalling fails, return an error
		return &pb.ForgotPasswordResponse{
			Success: false,
			Error:   "Something went wrong. Please try again later.",
		}, err
	}

	err = s.redisClient.Set(ctx, redisKey, payloadBytes, 5*time.Minute)
	if err != nil {
		// if the redis set fails, return an error
		return &pb.ForgotPasswordResponse{
			Success: false,
			Error:   "Something went wrong. Please try again later.",
		}, err
	}

	// create the email body
	// do not change format
	emailBody := fmt.Sprintf(`Hi there,

You requested a password reset.

To reset your password, enter the following 6-digit code:

Your verification code: %s

This code will expire in 5 minutes. If you did not request a password reset, please ignore this email.

Thank you,
The Indeq Team
`, otp)
	emailSubject := "Indeq - Reset Your Password"

	if err := s.sendEmail(req.Email, emailSubject, emailBody); err != nil {
		// if the email sending fails, return an error
		return &pb.ForgotPasswordResponse{
			Success: false,
			Error:   "Something went wrong. Please try again later.",
		}, err
	}

	return &pb.ForgotPasswordResponse{
		Success: true,
		Token:   token,
	}, nil
}

// rpc(context, reset password request)
//   - takes in a token and a password and resets the password
//   - returns true on success
func (s *authServer) ResetPassword(ctx context.Context, req *pb.ResetPasswordRequest) (*pb.ResetPasswordResponse, error) {
	if req.Token == "" {
		// if the token is empty, return an error
		return &pb.ResetPasswordResponse{
			Success: false,
			Error:   "No token found",
		}, nil
	}

	if req.Password == "" {
		// if the password is empty, return an error
		return &pb.ResetPasswordResponse{
			Success: false,
			Error:   "Password is required",
		}, nil
	}

	redisKey := fmt.Sprintf("forgot:%s", req.Token)
	data, err := s.redisClient.Get(ctx, redisKey)
	if err != nil {
		// if the redis get fails, return an error
		return &pb.ResetPasswordResponse{
			Success: false,
			Error:   "Something went wrong. Please try again later.",
		}, err
	}

	if data == "" {
		// if the data is empty, return an error
		return &pb.ResetPasswordResponse{
			Success: false,
			Error:   "No data found",
		}, nil
	}

	var payload ForgotPayload
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		// if the payload unmarshalling fails, return an error
		return &pb.ResetPasswordResponse{
			Success: false,
			Error:   "Something went wrong. Please try again later.",
		}, err
	}

	if err := s.validatePassword(req.Password); err != nil {
		// if the password is not valid, return an error
		return &pb.ResetPasswordResponse{
			Success: false,
			Error:   "Password is not valid",
		}, err
	}

	// Generate a random salt
	salt := make([]byte, s.argonParams.saltLength)
	if _, err := rand.Read(salt); err != nil {
		// if the salt generation fails, return an error
		return &pb.ResetPasswordResponse{
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

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		// if the transaction fails, return an error
		return &pb.ResetPasswordResponse{
			Success: false,
			Error:   "Something went wrong. Please try again later.",
		}, err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, "UPDATE users SET password_hash = $1 WHERE email = $2", encodedHash, strings.ToLower(payload.Email))
	if err != nil {
		// if the query fails, return an error
		return &pb.ResetPasswordResponse{
			Success: false,
			Error:   "Something went wrong. Please try again later.",
		}, err
	}

	if err := tx.Commit(); err != nil {
		// if the transaction fails, return an error
		return &pb.ResetPasswordResponse{
			Success: false,
			Error:   "Something went wrong. Please try again later.",
		}, err
	}

	// Delete the token from redis
	s.redisClient.Del(ctx, redisKey)

	return &pb.ResetPasswordResponse{
		Success: true,
	}, nil
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
            email VARCHAR(255) UNIQUE NOT NULL,
            password_hash TEXT NOT NULL,
            name VARCHAR(255) NOT NULL,
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

	<-sigChan // TODO: implement worker groups
	log.Print("gracefully shutting down...")
}
