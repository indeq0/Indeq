package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	pb "github.com/cc-0000/indeq/common/api"
	"github.com/golang-jwt/jwt/v5"
)

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

	var userCreated bool = false
	var userId string

	// Try to find user by email if provided and not empty
	userFoundByEmail := false
	userFoundByGoogleId := false
	if req.Email != "" {
		userId, _, _, _, _, _, err := getUserByEmail(ctx, tx, strings.ToLower(req.Email))
		if err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("database error when checking email: %v", err)
		} else if err == nil && userId != "" {
			userFoundByEmail = true
		}
	}
	if !userFoundByEmail {
		// try to find user by google_id
		userId, _, _, _, _, _, _, err := getUserByGoogleId(ctx, tx, req.GoogleId)
		if err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("database error when checking google_id: %v", err)
		} else if err == nil && userId != "" {
			userFoundByGoogleId = true
		}
	}

	if userFoundByEmail {
		// user already has an email --> add the google stuff to the entry
		userId, passwordHash, _, alias, avatarNum, _, err := getUserByEmail(ctx, tx, strings.ToLower(req.Email))
		if err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("database error when checking email: %v", err)
		} 
		updateUser(ctx, tx, userId, strings.ToLower(req.Email), sql.NullString{String: passwordHash, Valid: passwordHash != ""}, req.Name, alias, avatarNum, sql.NullString{String: req.GoogleId, Valid: true})
	} else if userFoundByGoogleId {
		// user already has a google --> update the google stuff to the entry
		userId, _, passwordHash, _, alias, avatarNum, _, err := getUserByGoogleId(ctx, tx, req.GoogleId)
		if err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("database error when checking google_id: %v", err)
		} 
		updateUser(ctx, tx, userId, strings.ToLower(req.Email), sql.NullString{String: passwordHash, Valid: passwordHash != ""}, req.Name, alias, avatarNum, sql.NullString{String: req.GoogleId, Valid: true})
	} else {
		// user has neither --> create a new entry with just the google stuff
		userId, err := createUser(ctx, tx, strings.ToLower(req.Email), "", req.Name, req.GoogleId)
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

// func(ctx context.Context, req *pb.SSOConnectRequest) (*pb.SSOConnectResponse, error)
//   - Handles Google SSO login flow. Exchanges authorization code for access token,
//     retrieves user info from Google, validates state, and creates/updates the user in the authentication service.
//   - Returns a populated SSOConnectResponse with a JWT token and user information on success.
//   - Returns an error if any step of the OAuth2 or user creation process fails.
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