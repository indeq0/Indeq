package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	pb "github.com/cc-0000/indeq/common/api"
	"github.com/cc-0000/indeq/common/config"
	"github.com/cc-0000/indeq/common/redis"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type integrationServer struct {
	pb.UnimplementedIntegrationServiceServer
	crawlingConn    *grpc.ClientConn
	crawlingService pb.CrawlingServiceClient
	db              *sql.DB // integration database
	redisClient     *redis.RedisClient
}

// TokenResponse represents the OAuth token response from our providers
type TokenResponse struct {
	AccessToken     string    `json:"access_token"`
	RefreshToken    string    `json:"refresh_token,omitempty"`
	ExpiresIn       int       `json:"expires_in"`
	TokenType       string    `json:"token_type"`
	Scope           string    `json:"scope"`
	ExpiresAt       time.Time `json:"-"`
	RequiresRefresh bool      `json:"-"`
}

// OAuthProviderConfig represents the token exchange and refresh
type OAuthProviderConfig struct {
	TokenURL     string
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

type OAuthToken struct {
	UserID                string
	Provider              string
	EncryptedRefreshToken string
	ExpiresAt             time.Time
	RequiresRefresh       bool
}

type RefreshedToken struct {
	UserID          string
	Provider        string
	AccessToken     string
	RefreshToken    string
	OldRefreshToken string
	ExpiresAt       time.Time
	RequiresRefresh bool
}

// OAuth provider configurations
var providers = map[string]OAuthProviderConfig{
	"GOOGLE": {
		TokenURL:     "https://oauth2.googleapis.com/token",
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		RedirectURI:  os.Getenv("GOOGLE_REDIRECT_URI"),
	},
	"MICROSOFT": {
		TokenURL:     "https://login.microsoftonline.com/common/oauth2/v2.0/token",
		ClientID:     os.Getenv("MICROSOFT_CLIENT_ID"),
		ClientSecret: os.Getenv("MICROSOFT_CLIENT_SECRET"),
		RedirectURI:  os.Getenv("MICROSOFT_REDIRECT_URI"),
	},
	"NOTION": {
		TokenURL:     "https://api.notion.com/v1/oauth/token",
		ClientID:     os.Getenv("NOTION_CLIENT_ID"),
		ClientSecret: os.Getenv("NOTION_CLIENT_SECRET"),
		RedirectURI:  os.Getenv("NOTION_REDIRECT_URI"),
	},
}

// Exchanges an auth code for access & refresh token
func ExchangeAuthCodeForToken(provider string, authCode string) (*TokenResponse, error) {
	config, exists := providers[provider]
	if !exists {
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
	log.Printf("Exchanging auth code for token with provider: %s and auth code: %s", provider, authCode)

	var req *http.Request
	var err error

	if provider == "NOTION" {
		authHeader := "Basic " + base64.StdEncoding.EncodeToString(
			[]byte(config.ClientID+":"+config.ClientSecret))

		data := map[string]string{
			"grant_type":   "authorization_code",
			"code":         authCode,
			"redirect_uri": config.RedirectURI,
		}

		jsonData, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}

		req, err = http.NewRequest("POST", config.TokenURL, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, err
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)
	} else {
		data := fmt.Sprintf(
			"client_id=%s&client_secret=%s&code=%s&grant_type=authorization_code&redirect_uri=%s",
			config.ClientID, config.ClientSecret, authCode, config.RedirectURI,
		)
		req, err = http.NewRequest("POST", config.TokenURL, strings.NewReader(data))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to exchange auth code: %s", resp.Status)
	}

	var tokenRes TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenRes); err != nil {
		return nil, err
	}
	if provider != "NOTION" {
		tokenRes.ExpiresAt = time.Now().Add(time.Duration(tokenRes.ExpiresIn) * time.Second)
		tokenRes.RequiresRefresh = true
	} else {
		tokenRes.RequiresRefresh = false
	}
	return &tokenRes, nil
}

func Encrypt(plaintext string) (string, error) {
	base64Key := os.Getenv("ENCRYPTION_KEY")

	if base64Key == "" {
		return "", fmt.Errorf("ENCRYPTION_KEY is not set")
	}

	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return "", fmt.Errorf("failed to decode encryption key: %v", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %v", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %v", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %v", err)
	}

	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func Decrypt(ciphertext string) (string, error) {
	base64Key := os.Getenv("ENCRYPTION_KEY")
	if base64Key == "" {
		return "", fmt.Errorf("ENCRYPTION_KEY is not set")
	}

	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return "", fmt.Errorf("failed to decode encryption key: %v", err)
	}

	decodedCiphertext, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %v", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %v", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %v", err)
	}

	nonceSize := aesGCM.NonceSize()
	if len(decodedCiphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, encryptedData := decodedCiphertext[:nonceSize], decodedCiphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, encryptedData, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt ciphertext: %v", err)
	}

	return string(plaintext), nil
}

// Refresh an expired access token
func RefreshOAuthToken(provider string, refreshToken string) (*TokenResponse, error) {
	config, exists := providers[provider]
	if !exists {
		return nil, errors.New("unsupported provider")
	}

	data := fmt.Sprintf(
		"client_id=%s&client_secret=%s&refresh_token=%s&grant_type=refresh_token",
		config.ClientID, config.ClientSecret, refreshToken,
	)

	req, err := http.NewRequest("POST", config.TokenURL, strings.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to refresh token: %s", resp.Status)
	}

	var tokenRes TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenRes); err != nil {
		return nil, err
	}

	tokenRes.ExpiresAt = time.Now().Add(time.Duration(tokenRes.ExpiresIn) * time.Second)
	return &tokenRes, nil
}

func startTokenRefreshWorker(db *sql.DB) {
	ticker := time.NewTicker(time.Minute)
	go func() {
		for range ticker.C {
			log.Println("Refreshing all expired tokens...")
			err := refreshAllExpiredTokens(db)
			if err != nil {
				log.Printf("Error refreshing all expired tokens: %v", err)
			}
		}
	}()
}

func refreshAllExpiredTokens(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var tokensToRefresh []OAuthToken
	rows, err := db.QueryContext(ctx, `
		SELECT user_id, provider, refresh_token, expires_at, requires_refresh
		FROM oauth_tokens
		WHERE expires_at < NOW() + INTERVAL '5 minutes'
		AND requires_refresh = TRUE
	`)
	if err != nil {
		return fmt.Errorf("failed to query expired tokens: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var token OAuthToken
		if err := rows.Scan(
			&token.UserID,
			&token.Provider,
			&token.EncryptedRefreshToken,
			&token.ExpiresAt,
			&token.RequiresRefresh,
		); err != nil {
			return fmt.Errorf("failed to scan token row: %v", err)
		}
		tokensToRefresh = append(tokensToRefresh, token)
	}
	rows.Close()

	if len(tokensToRefresh) == 0 {
		log.Println("No expired tokens to refresh")
		return nil
	}

	var refreshedTokens []RefreshedToken
	for _, token := range tokensToRefresh {
		if token.EncryptedRefreshToken == "" || !token.RequiresRefresh {
			log.Printf("No refresh token found for user %s and provider %s", token.UserID, token.Provider)
			continue
		}

		// decrypt refresh token
		decryptedRefreshToken, err := Decrypt(token.EncryptedRefreshToken)
		if err != nil {
			log.Printf("Error decrypting refresh token for user %s: %v", token.UserID, err)
			continue
		}

		// refresh token
		tokenData, err := RefreshOAuthToken(token.Provider, decryptedRefreshToken)
		if err != nil {
			log.Printf("Error refreshing token for user %s: %v", token.UserID, err)
			continue
		}

		if tokenData.RefreshToken == "" {
			log.Printf("No new refresh token returned for user %s; reusing old one", token.UserID)
			tokenData.RefreshToken = decryptedRefreshToken
		}

		// encrypt new tokens before saving
		encryptedAccessToken, err := Encrypt(tokenData.AccessToken)
		if err != nil {
			log.Printf("Error encrypting access token for user %s: %v", token.UserID, err)
			continue
		}

		encryptedRefreshToken, err := Encrypt(tokenData.RefreshToken)
		if err != nil {
			log.Printf("Error encrypting refresh token for user %s: %v", token.UserID, err)
			continue
		}

		refreshedTokens = append(refreshedTokens, RefreshedToken{
			UserID:          token.UserID,
			Provider:        token.Provider,
			AccessToken:     encryptedAccessToken,
			RefreshToken:    encryptedRefreshToken,
			OldRefreshToken: token.EncryptedRefreshToken,
			ExpiresAt:       tokenData.ExpiresAt,
			RequiresRefresh: tokenData.RequiresRefresh,
		})
	}

	// perform all updates in one transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}

	// commit or rollback in a deferred function
	defer func() {
		if p := recover(); p != nil {
			// if we have panicked, rollback
			_ = tx.Rollback()
			panic(p)
		} else if err != nil {
			// if the transaction failed, rollback
			_ = tx.Rollback()
		} else {
			// if we succeeded, commit
			commitErr := tx.Commit()
			if commitErr != nil {
				log.Printf("Failed to commit transaction: %v", commitErr)
			}
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `
		UPDATE oauth_tokens
		SET access_token = $1, refresh_token = $2, expires_at = $3, updated_at = NOW(), requires_refresh = $4
		WHERE user_id = $5 AND provider = $6
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare update statement: %v", err)
	}
	defer stmt.Close()

	for _, token := range refreshedTokens {
		_, err := stmt.ExecContext(
			ctx,
			token.AccessToken,
			token.RefreshToken,
			token.ExpiresAt,
			token.RequiresRefresh,
			token.UserID,
			token.Provider,
		)
		if err != nil {
			log.Printf("Error updating token for user %s: %v", token.UserID, err)
			return fmt.Errorf("error updating token for user %s: %v", token.UserID, err)
		}

		if token.RefreshToken != token.OldRefreshToken {
			log.Printf("Successfully refreshed access token and refresh token for user %s", token.UserID)
		} else {
			log.Printf("Successfully refreshed the access token for user %s", token.UserID)
		}
	}

	return nil
}

func enumToString(provider pb.Provider) (string, error) {
	switch provider {
	case pb.Provider_GOOGLE:
		return "GOOGLE", nil
	case pb.Provider_MICROSOFT:
		return "MICROSOFT", nil
	case pb.Provider_NOTION:
		return "NOTION", nil
	default:
		return "", fmt.Errorf("invalid provider: %v", provider)
	}
}

func generateState(provider string) (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	stateData := append([]byte(provider), b...)
	state := base64.URLEncoding.EncodeToString(stateData)
	return state, nil
}

func (s *integrationServer) GetOAuthURL(ctx context.Context, req *pb.GetOAuthURLRequest) (*pb.GetOAuthURLResponse, error) {
	providerStr, err := enumToString(req.Provider)
	if err != nil {
		return nil, fmt.Errorf("failed to convert provider to string: %v", err)
	}

	state, err := generateState(providerStr)
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %v", err)
	}
	err = s.redisClient.StoreOAuthState(ctx, state, req.UserId)
	if err != nil {
		return nil, fmt.Errorf("failed to store oauth state: %v", err)
	}
	var authURL string
	params := url.Values{}
	if providerStr == "NOTION" {
		authURL = os.Getenv("NOTION_AUTH_URL") + "&state=" + state
	} else {
		params.Add("response_type", "code")
		params.Add("state", state)
		if providerStr == "GOOGLE" {
			params.Add("access_type", "offline")
			params.Add("prompt", "consent")
			params.Add("redirect_uri", os.Getenv("GOOGLE_REDIRECT_URI"))
			params.Add("scope", os.Getenv("GOOGLE_SCOPES"))
			params.Add("client_id", os.Getenv("GOOGLE_CLIENT_ID"))
			authURL = os.Getenv("GOOGLE_AUTH_URL") + "?" + params.Encode()
		} else if providerStr == "MICROSOFT" {
			params.Add("response_mode", "query")
			params.Add("redirect_uri", os.Getenv("MICROSOFT_REDIRECT_URI"))
			params.Add("scope", os.Getenv("MICROSOFT_SCOPES"))
			params.Add("client_id", os.Getenv("MICROSOFT_CLIENT_ID"))
			authURL = os.Getenv("MICROSOFT_AUTH_URL") + "?" + params.Encode()
		}
	}

	return &pb.GetOAuthURLResponse{
		Url: authURL,
	}, nil
}

func (s *integrationServer) GetIntegrations(ctx context.Context, req *pb.GetIntegrationsRequest) (*pb.GetIntegrationsResponse, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT provider
		FROM oauth_tokens
		WHERE user_id = $1
	`, req.UserId)

	if err != nil {
		return nil, fmt.Errorf("failed to query integrations: %v", err)
	}
	defer rows.Close()

	providersSet := make(map[pb.Provider]bool)
	for rows.Next() {
		var provider string
		if err := rows.Scan(&provider); err != nil {
			return nil, fmt.Errorf("failed to scan provider: %v", err)
		}

		switch provider {
		case "GOOGLE":
			providersSet[pb.Provider_GOOGLE] = true
		case "MICROSOFT":
			providersSet[pb.Provider_MICROSOFT] = true
		case "NOTION":
			providersSet[pb.Provider_NOTION] = true
		}
	}

	var providers []pb.Provider
	for provider := range providersSet {
		providers = append(providers, provider)
	}

	return &pb.GetIntegrationsResponse{
		Providers: providers,
	}, nil
}

func (s *integrationServer) ConnectIntegration(ctx context.Context, req *pb.ConnectIntegrationRequest) (*pb.ConnectIntegrationResponse, error) {
	providerStr, err := enumToString(req.Provider)
	if err != nil {
		return &pb.ConnectIntegrationResponse{
			Success:      false,
			Message:      fmt.Sprintf("Failed to convert provider to string: %v", err),
			ErrorDetails: err.Error(),
		}, nil
	}

	tokenRes, err := ExchangeAuthCodeForToken(providerStr, req.AuthCode)
	if err != nil {
		return &pb.ConnectIntegrationResponse{
			Success:      false,
			Message:      fmt.Sprintf("Failed to exchange auth code: %v", err),
			ErrorDetails: err.Error(),
		}, nil
	}

	encryptedAccessToken, err := Encrypt(tokenRes.AccessToken)
	if err != nil {
		return &pb.ConnectIntegrationResponse{
			Success:      false,
			Message:      fmt.Sprintf("Failed to encrypt access token: %v", err),
			ErrorDetails: err.Error(),
		}, nil
	}

	encryptedRefreshToken, err := Encrypt(tokenRes.RefreshToken)
	if err != nil {
		return &pb.ConnectIntegrationResponse{
			Success:      false,
			Message:      fmt.Sprintf("Failed to encrypt refresh token: %v", err),
			ErrorDetails: err.Error(),
		}, nil
	}

	_, err = s.db.ExecContext(ctx, `
	INSERT INTO oauth_tokens (user_id, provider, access_token, refresh_token, expires_at, requires_refresh)
	VALUES ($1, $2, $3, $4, $5, $6)
	ON CONFLICT (user_id, provider)
	DO UPDATE SET
		access_token = EXCLUDED.access_token,
		refresh_token = COALESCE(NULLIF(EXCLUDED.refresh_token, ''), oauth_tokens.refresh_token),
		expires_at = EXCLUDED.expires_at,
		updated_at = NOW(),
		requires_refresh = EXCLUDED.requires_refresh
	`,
		req.UserId,
		providerStr,
		encryptedAccessToken,
		encryptedRefreshToken,
		tokenRes.ExpiresAt,
		tokenRes.RequiresRefresh,
	)

	if err != nil {
		return &pb.ConnectIntegrationResponse{
			Success:      false,
			Message:      fmt.Sprintf("Database error saving token: %v", err),
			ErrorDetails: err.Error(),
		}, nil
	}

	go func() {
		bgCtx := context.Background()
		crawlerReq := &pb.StartInitalCrawlerRequest{
			UserId:      req.UserId,
			Platform:    providerStr,
			AccessToken: tokenRes.AccessToken,
		}
		_, err = s.crawlingService.StartInitialCrawler(bgCtx, crawlerReq)
		if err != nil {
			log.Printf("Failed to start initial crawler: %v", err)
		}
	}()

	return &pb.ConnectIntegrationResponse{
		Success: true,
		Message: "Integration connected successfully",
	}, nil
}

func (s *integrationServer) DisconnectIntegration(ctx context.Context, req *pb.DisconnectIntegrationRequest) (*pb.DisconnectIntegrationResponse, error) {
	providerStr, err := enumToString(req.Provider)
	if err != nil {
		return &pb.DisconnectIntegrationResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to convert provider to string: %v", err),
		}, nil
	}

	result, err := s.db.ExecContext(ctx, `
	DELETE FROM oauth_tokens
	WHERE user_id = $1 AND provider = $2
	`,
		req.UserId,
		providerStr,
	)

	if err != nil {
		return &pb.DisconnectIntegrationResponse{
			Success: false,
			Message: fmt.Sprintf("Database error deleting token: %v", err),
		}, nil
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return &pb.DisconnectIntegrationResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to get rows affected: %v", err),
		}, nil
	}

	if rowsAffected == 0 {
		return &pb.DisconnectIntegrationResponse{
			Success: false,
			Message: "No token found to delete",
		}, nil
	}

	_, err = s.crawlingService.DeleteCrawlerData(ctx, &pb.DeleteCrawlerDataRequest{
		UserId:   req.UserId,
		Platform: providerStr,
	})
	if err != nil {
		log.Printf("Failed to delete crawler data: %v", err)
	}

	return &pb.DisconnectIntegrationResponse{
		Success: true,
		Message: "Integration disconnected successfully",
	}, nil
}

func (s *integrationServer) ValidateOAuthState(ctx context.Context, req *pb.ValidateOAuthStateRequest) (*pb.ValidateOAuthStateResponse, error) {
	userId, err := s.redisClient.ValidateOAuthState(ctx, req.State)
	if err != nil {
		return &pb.ValidateOAuthStateResponse{
			Success:      false,
			ErrorDetails: err.Error(),
		}, nil
	}

	return &pb.ValidateOAuthStateResponse{
		Success: true,
		UserId:  userId,
	}, nil
}

func (s *integrationServer) GetAccessToken(ctx context.Context, req *pb.GetAccessTokenRequest) (*pb.GetAccessTokenResponse, error) {
	providerStr, err := enumToString(req.Platform)
	if err != nil {
		return &pb.GetAccessTokenResponse{
			Success:      false,
			Message:      fmt.Sprintf("Failed to convert provider to string: %v", err),
			ErrorDetails: err.Error(),
		}, nil
	}

	// Query the database to get the encrypted access token
	var encryptedAccessToken string
	err = s.db.QueryRowContext(ctx, `
		SELECT access_token
		FROM oauth_tokens
		WHERE user_id = $1 AND provider = $2
	`, req.UserId, providerStr).Scan(&encryptedAccessToken)

	if err == sql.ErrNoRows {
		return &pb.GetAccessTokenResponse{
			Success:      false,
			Message:      "No access token found for the given user and provider",
			ErrorDetails: "Not found",
		}, nil
	} else if err != nil {
		return &pb.GetAccessTokenResponse{
			Success:      false,
			Message:      fmt.Sprintf("Database error retrieving token: %v", err),
			ErrorDetails: err.Error(),
		}, nil
	}

	// Decrypt the access token
	decryptedAccessToken, err := Decrypt(encryptedAccessToken)
	if err != nil {
		return &pb.GetAccessTokenResponse{
			Success:      false,
			Message:      fmt.Sprintf("Failed to decrypt access token: %v", err),
			ErrorDetails: err.Error(),
		}, nil
	}

	return &pb.GetAccessTokenResponse{
		Success:     true,
		AccessToken: decryptedAccessToken,
		Message:     "Access token retrieved successfully",
	}, nil
}

func main() {
	log.Println("Starting the integration server...")

	// Load all environmetal variables

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatalf("DATABASE_URL envionment variable is required")
	}

	// Load the TLS configuration values
	tlsConfig, err := config.LoadServerTLSFromEnv("INTEGRATION_CRT", "INTEGRATION_KEY")
	if err != nil {
		log.Fatal("Error loading TLS config for integration service")
	}

	// Load the TLS configuration values for crawling service
	crawlingTLSConfig, err := config.LoadClientTLSFromEnv("CRAWLING_CRT", "CRAWLING_KEY", "CA_CRT")
	if err != nil {
		log.Fatal("Error loading TLS client config for crawling service")
	}

	// Connect to crawling service
	crawlingAddress := os.Getenv("CRAWLING_ADDRESS")
	if crawlingAddress == "" {
		log.Fatalf("CRAWLING_ADDRESS environment variable is required")
	}

	crawlingConn, err := grpc.NewClient(
		crawlingAddress,
		grpc.WithTransportCredentials(credentials.NewTLS(crawlingTLSConfig)),
	)
	if err != nil {
		log.Fatalf("Failed to connect to crawling service: %v", err)
	}
	defer crawlingConn.Close()

	crawlingService := pb.NewCrawlingServiceClient(crawlingConn)

	// Connect to db
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = db.ExecContext(ctx, `
        CREATE TABLE IF NOT EXISTS oauth_tokens (
            id SERIAL PRIMARY KEY,
            user_id UUID NOT NULL,
            provider TEXT NOT NULL CHECK (provider IN ('GOOGLE', 'MICROSOFT', 'NOTION')),
            access_token TEXT NOT NULL,
            refresh_token TEXT NULL,
            expires_at TIMESTAMP WITH TIME ZONE NULL,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			requires_refresh BOOLEAN DEFAULT TRUE,
			UNIQUE (user_id, provider)
        );
    `)

	if err != nil {
		log.Fatalf("Failed to create oauth_tokens table: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS user_provider_idx ON oauth_tokens (user_id, provider);
	`)
	if err != nil {
		log.Fatalf("Failed to create user_provider index: %v", err)
	}

	fmt.Println("Database setup completed: oauth_tokens table is ready.")
	startTokenRefreshWorker(db)

	// Pull in GRPC address
	grpcAddress := os.Getenv("INTEGRATION_PORT")
	if grpcAddress == "" {
		log.Fatalf("INTEGRATION_PORT environment variable is required")
	}

	listener, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	log.Println("Creating the integration server")

	opts := []grpc.ServerOption{
		grpc.Creds(credentials.NewTLS(tlsConfig)),
	}

	redisClient, err := redis.NewRedisClient(context.Background(), 0)
	if err != nil {
		log.Fatalf("Failed to connect to redis: %v", err)
	}
	defer redisClient.Client.Close()

	grpcServer := grpc.NewServer(opts...)
	pb.RegisterIntegrationServiceServer(grpcServer, &integrationServer{
		db:              db,
		redisClient:     redisClient,
		crawlingConn:    crawlingConn,
		crawlingService: crawlingService,
	})
	log.Printf("Integration Service listening on %v\n", listener.Addr())
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	} else {
		log.Printf("Integration Service served on %v\n", listener.Addr())
	}
}
