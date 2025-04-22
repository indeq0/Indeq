package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	pb "github.com/cc-0000/indeq/common/api"
)

func corsMiddleware(next http.Handler) http.Handler {
	// establishes site-wide CORS policies
	allowedIp, ok := os.LookupEnv("ALLOWED_CLIENT_IP")
	if !ok {
		log.Fatal("Failed to retrieve ALLOWED_CLIENT_IP")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedIp)

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Max-Age", "3600") // tell the browser to cache the pre-flight request for 3600 seconds aka an hour
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func authMiddleware(next http.HandlerFunc, clients *ServiceClients) http.HandlerFunc {
	// simply modifies a handler func to pass these checks first
	return func(w http.ResponseWriter, r *http.Request) {
		var authToken string
		// Check if this is a WebSocket upgrade request
		if r.Header.Get("Upgrade") == "websocket" {
			// Get token from Sec-WebSocket-Protocol header (known workaround)
			// *websocket doesn't have explicit auth header setting support
			protocols := r.Header.Get("Sec-WebSocket-Protocol")

			// Split the protocol value by comma
			parts := strings.Split(protocols, ",")
			for i, part := range parts {
				// Trim spaces from each part
				parts[i] = strings.TrimSpace(part)
			}

			if len(parts) >= 2 && parts[0] == "Authorization" {
				authToken = parts[1]
			}

			// Set the selected protocol in response <-- client needs this
			if authToken != "" {
				w.Header().Set("Sec-WebSocket-Protocol", "Authorization")
			}
		} else {
			// Regular HTTP request - use Authorization header
			auth_header := r.Header.Get("Authorization")
			if auth_header == "" {
				http.Error(w, "No authorization token provided", http.StatusUnauthorized)
				return
			}
			authToken = strings.TrimPrefix(auth_header, "Bearer ")
		}

		res, err := clients.authClient.Verify(r.Context(), &pb.VerifyRequest{
			Token: authToken,
		})

		if err != nil || !res.Valid {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r) // if they pass the checks serve the next handler
	}
}
