package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	pb "github.com/cc-0000/indeq/common/api"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
)

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

const (
	subjectVerify = "Indeq - Verify Your Account"
	subjectReset  = "Indeq - Reset Your Password"
	verifyBody    = `Welcome to Indeq!

To verify your account, enter the following 6-digit code:

Your verification code: %s

This code will expire in 5 minutes. If you did not request this verification code, you can safely ignore this email.

Thank you,
The Indeq Team
`
	resetBody = `Hi there,

You requested a password reset.

To reset your password, enter the following 6-digit code:

Your verification code: %s

This code will expire in 5 minutes. If you did not request a password reset, please ignore this email.

Thank you,
The Indeq Team`
)

// rpc(context, login request)
//   - takes a username and password and tries to find a matching entry in our user database
//   - fails if rate limited or (user, password) is not a match
//   - returns a new JWT and user id on success
func (s *authServer) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	// Rate limit
	if exceeded, err := s.checkRateLimit(ctx, req.Email); err != nil {
		return nil, err
	} else if exceeded {
		return &pb.LoginResponse{}, fmt.Errorf("too many attempts, please try again later")
	}

	// get the id and encoded password hash matching user email
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	id, encodedHash, name, alias, avatarNum, err := getUserByEmail(ctx, tx, strings.ToLower(req.Email))
	if err == sql.ErrNoRows {
		// Even though the user doesn't exist we want to fake a comparison
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
		return &pb.LoginResponse{}, fmt.Errorf("invalid credentials")
	}
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	match, err := comparePasswordAndEncodedHash(req.Password, encodedHash)
	if err != nil {
		return &pb.LoginResponse{}, fmt.Errorf("invalid credentials")
	}
	if !match {
		// Increment failed attempts counter
		s.incrementFailedAttempts(ctx, req.Email)
		return &pb.LoginResponse{}, fmt.Errorf("invalid credentials")
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

	return &pb.LoginResponse{Token: tokenString, UserId: id, Name: name, Alias: alias, AvatarNum: int32(avatarNum)}, nil
}

// rpc(context, register request)
//   - takes in a email, name and password and creates a temporary redis entry with that information awaiting OTP approval
//   - email, password must pass validation
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

		// link to google account here
		print("User not created, trying to find existing user")

		// check if userID
		var userId string
		var googleId string
		var passwordHash sql.NullString
		err = s.db.QueryRowContext(
			ctx,
			"SELECT id, google_id, password_hash FROM users WHERE email = $1",
			strings.ToLower(req.Email), // Normalize email
		).Scan(&userId, &googleId, &passwordHash)

		if err != sql.ErrNoRows {

			// Google ID exists without a password hash
			if err == nil && googleId != "" && (passwordHash.String == "") {
				tx, err := s.db.BeginTx(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to begin transaction: %v", err)
				}
				defer tx.Rollback()

				err = tx.QueryRowContext(
					ctx,
					"UPDATE users SET password_hash = $1 WHERE email = $2 RETURNING id",
					passwordHash,
					strings.ToLower(req.Email),
				).Scan()
				if err := tx.Commit(); err != nil {
					return nil, fmt.Errorf("failed to commit transaction: %v", err)
				}

				var currentTime = time.Now()

				token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
					"sub": userId,
					"exp": currentTime.Add(24 * time.Hour).Unix(), // current 1 day expiration
					"iat": currentTime.Unix(),
					"nbf": currentTime.Unix(),
				})

				tokenString, err := token.SignedString(s.jwtSecret)

				return &pb.RegisterResponse{Success: true, Error: "Linked to Existing Google Account", Token: tokenString}, nil
			}
		}

		return &pb.RegisterResponse{
			Success: false,
			Error:   "Email already exists!",
		}, nil
	}

	encodedHash, err := saltAndHashPassword(req.Password, s.argonParams)
	if err != nil {
		return &pb.RegisterResponse{
			Success: false,
			Error:   "Something went wrong. Please try again later.",
		}, err
	}

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
	emailBody := fmt.Sprintf(verifyBody, otp)

	err = s.sendEmail(req.Email, subjectVerify, emailBody)
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
		emailBody := fmt.Sprintf(verifyBody, newOTP)

		if err := s.sendEmail(payload.Email, subjectVerify, emailBody); err != nil {
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
		emailBody := fmt.Sprintf(resetBody, newOTP)
		// Send the email.
		if err := s.sendEmail(payload.Email, subjectReset, emailBody); err != nil {
			// if the email sending fails, return an error
			return &pb.ResendOTPResponse{
				Success: false,
				Error:   "Failed to send password reset email",
			}, err
		}
	}

	return &pb.ResendOTPResponse{
		Success: true,
	}, nil
}

// rpc(context, verify otp request)
//   - takes in a token and a code and verifies the code
//   - if the type is register, it will store the user in the database
//   - if the type is forgot, it will update the user's password
//   - creates corresponding Vector, and Desktop datastores for the user
//   - returns: user id and true, or error on failure
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

		userId, err := createUser(ctx, tx, strings.ToLower(payload.Email), payload.HashedPassword, payload.Name)
		if err != nil {

			// otherwise return an error
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
	emailBody := fmt.Sprintf(resetBody, otp)

	if err := s.sendEmail(req.Email, subjectReset, emailBody); err != nil {
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

	// Generate a password hash
	encodedHash, err := saltAndHashPassword(req.Password, s.argonParams)
	if err != nil {
		// if the password hashing fails, return an error
		return &pb.ResetPasswordResponse{
			Success: false,
			Error:   "Something went wrong. Please try again later.",
		}, err
	}

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

// rpc(context, set user account settings request)
//   - takes a user id, name, alias, and avatar number, and updates the user's name, alias, and avatar in the database
//   - returns an empty response on success, or error on failure
func (s *authServer) SetUserAccountSettings(ctx context.Context, req *pb.SetUserAccountSettingsRequest) (*pb.SetUserAccountSettingsResponse, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return &pb.SetUserAccountSettingsResponse{}, err
	}
	defer tx.Rollback()

	email, passwordHash, name, alias, avatarNum, err := getUserById(ctx, tx, req.UserId)
	if err != nil {
		return &pb.SetUserAccountSettingsResponse{}, err
	}

	// perform updates on the existing values only if the values are not empty strings
	if (req.Name != "") {
		name = req.Name
	}
	if (req.Alias != "") {
		alias = req.Alias
	}
	if (req.AvatarNum != 0) {
		avatarNum = int(req.AvatarNum)
	}

	_, err = updateUser(ctx, tx, req.UserId, email, passwordHash, name, alias, avatarNum)
	if err != nil {
		return &pb.SetUserAccountSettingsResponse{}, err
	}

	if err := tx.Commit(); err != nil {
		return &pb.SetUserAccountSettingsResponse{}, err
	}

	return &pb.SetUserAccountSettingsResponse{}, nil
}

// rpc(context, get user account settings request)
//   - takes a user id and retrieves the user's alias from the database
//   - returns the alias string on success, or error on failure
func (s *authServer) GetUserAccountSettings(ctx context.Context, req *pb.GetUserAccountSettingsRequest) (*pb.GetUserAccountSettingsResponse, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return &pb.GetUserAccountSettingsResponse{}, err
	}
	defer tx.Rollback()

	email, _, name, alias, avatarNum, err := getUserById(ctx, tx, req.UserId)
	if err != nil {
		return &pb.GetUserAccountSettingsResponse{}, err
	}

	if err := tx.Commit(); err != nil {
		return &pb.GetUserAccountSettingsResponse{}, err
	}

	return &pb.GetUserAccountSettingsResponse{
		Alias:   alias,
		Name:    name,
		Email:   email,
		AvatarNum: int32(avatarNum),
	}, nil
}

// rpc(context, delete account request)
//   - takes a user id and deletes the user's account from the database
//   - returns an empty response on success, or error on failure
func (s *authServer) DeleteAccount(ctx context.Context, req *pb.DeleteUserRequest) (*pb.DeleteUserResponse, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return &pb.DeleteUserResponse{}, err
	}
	defer tx.Rollback()

	// Delete all conversations
	conversations, err := s.queryService.GetAllConversations(ctx, &pb.QueryGetAllConversationsRequest{
		UserId: req.UserId,
	})
	if err != nil {
		return &pb.DeleteUserResponse{}, err
	}
	for _, conversation := range conversations.ConversationHeaders {
		_, err = s.queryService.DeleteConversation(ctx, &pb.QueryDeleteConversationRequest{
			UserId:         req.UserId,
			ConversationId: conversation.ConversationId,
		})
		if err != nil {
			return &pb.DeleteUserResponse{}, err
		}
	}

	// Delete all cloud related data
	res, err := s.integrationService.GetIntegrations(ctx, &pb.GetIntegrationsRequest{
		UserId: req.UserId,
	})
	if err != nil {
		return &pb.DeleteUserResponse{}, err
	}

	for _, provider := range res.Providers {
		_, err = s.integrationService.DisconnectIntegration(ctx, &pb.DisconnectIntegrationRequest{
			UserId:   req.UserId,
			Provider: provider,
		})
		if err != nil {
			return &pb.DeleteUserResponse{}, err
		}
	}

	// Delete all desktop related data
	_, err = s.desktopClient.DeleteUserData(ctx, &pb.DeleteUserRequest{
		UserId: req.UserId,
	})
	if err != nil {
		return &pb.DeleteUserResponse{}, err
	}

	// Delete the user from our auth stores
	err = deleteUser(ctx, tx, req.UserId)
	if err != nil {
		return &pb.DeleteUserResponse{}, err
	}

	if err := tx.Commit(); err != nil {
		return &pb.DeleteUserResponse{}, err
	}

	return &pb.DeleteUserResponse{}, nil
}
