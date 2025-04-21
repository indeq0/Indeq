package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// Platform and service constants
const (
	PlatformGoogle        = "GOOGLE"
	ServiceDrive          = "GOOGLE_DRIVE"
	ServiceGmail          = "GOOGLE_GMAIL"
	PlatformNotion        = "NOTION"
	ServiceNotion         = "NOTION"
	PlatformMicrosoft     = "MICROSOFT"
	ServiceMicrosoftDrive = "MICROSOFT_DRIVE"
)

// RetrievalToken represents a token entry in the database
type RetrievalToken struct {
	UserID         string
	Platform       string
	Service        string
	RetrievalToken string
	RequiresUpdate bool
}

type ChunkIDMapping struct {
	ShortKey   string `json:"shortKey"`   // Generated short key for the chunk
	ChunkID    string `json:"chunkId"`    // Original long chunk ID
	Service    string `json:"service"`    // Service-specific identifier
	ResourceID string `json:"resourceId"` // ID of the parent resource (e.g., page/database ID)
	CreatedAt  string `json:"createdAt"`  // Timestamp of creation
	LastUsed   string `json:"lastUsed"`   // Timestamp of last access
}

// Database operations
const (
	// RetrievalToken database operations
	insertRetrievalTokenQuery = `
		INSERT INTO retrievalTokens (
			user_id, platform, service, retrieval_token, 
			created_at, updated_at, requires_update
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, service)
		DO UPDATE SET 
			retrieval_token = EXCLUDED.retrieval_token,
			updated_at = EXCLUDED.updated_at,
			requires_update = EXCLUDED.requires_update
	`

	deleteRetrievalTokensQuery = `
		DELETE FROM retrievalTokens
		WHERE user_id = $1 AND platform = $2
	`

	getRetrievalTokensQuery = `
		SELECT platform, service, retrieval_token
		FROM retrievalTokens
		WHERE user_id = $1 AND platform = $2
		FOR UPDATE
	`

	getOutdatedTokensQuery = `
		SELECT user_id, platform, service, retrieval_token
		FROM retrievalTokens
		WHERE updated_at < NOW() - INTERVAL '1 minutes'
		AND requires_update = TRUE
		FOR UPDATE
	`

	// ProcessingStatus database operations

	upsertProcessingStatusQuery = `
		INSERT INTO processing_status (
			user_id, resource_id, platform, is_processed, crawling_done,
			created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, resource_id)
		DO UPDATE SET 
			is_processed = EXCLUDED.is_processed,
			updated_at = EXCLUDED.updated_at
	`

	updateCrawlingDoneQuery = `
		UPDATE processing_status
		SET crawling_done = $3,
			updated_at = $4
		WHERE user_id = $1 AND platform = $2
	`

	getProcessingStatusQuery = `
		SELECT resource_id, is_processed, crawling_done
		FROM processing_status
		WHERE user_id = $1 AND platform = $2
	`

	deleteProcessingStatusQuery = `
		DELETE FROM processing_status
		WHERE user_id = $1 AND platform = $2
	`
)

// setupDatabase creates and configures the database tables
func setupDatabase(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			// If an error occurred, rollback the transaction
			if rbErr := tx.Rollback(); rbErr != nil {
				log.Printf("tx rollback failed: %v", rbErr)
			}
		}
	}()

	// create retrievalTokens table
	_, err = tx.ExecContext(ctx, `
        CREATE TABLE IF NOT EXISTS retrievalTokens (
            id SERIAL PRIMARY KEY,
            user_id UUID NOT NULL,
			platform TEXT NOT NULL CHECK (platform IN ('GOOGLE', 'NOTION', 'MICROSOFT')),
            service TEXT NOT NULL CHECK (service IN ('GOOGLE_DRIVE', 'GOOGLE_GMAIL', 'NOTION', 'MICROSOFT_DRIVE')),
            retrieval_token TEXT NOT NULL,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			requires_update BOOLEAN DEFAULT TRUE,
			UNIQUE (user_id, service)
        );
    `)
	if err != nil {
		return fmt.Errorf("failed to create retrievalTokens table: %v", err) // Error will trigger rollback
	}
	_, err = tx.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS user_service_idx ON retrievalTokens (user_id, service);
	`)
	if err != nil {
		return fmt.Errorf("failed to create user_service index: %v", err) // Error will trigger rollback
	}

	// Create processing_status table
	_, err = tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS processing_status (
			id SERIAL PRIMARY KEY,
			user_id UUID NOT NULL,
			resource_id TEXT NOT NULL,
			platform TEXT NOT NULL CHECK (platform IN ('GOOGLE', 'NOTION', 'MICROSOFT')),
			is_processed BOOLEAN DEFAULT FALSE,
			crawling_done BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (user_id, resource_id)
		);
	`)
	if err != nil {
		return fmt.Errorf("failed to create processing_status table: %v", err) // Error will trigger rollback
	}

	_, err = tx.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS processing_status_user_idx ON processing_status (user_id);
	`)
	if err != nil {
		return fmt.Errorf("failed to create processing_status_user index: %v", err) // Error will trigger rollback
	}

	// Commit the transaction if all operations succeed
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	fmt.Println("Database setup completed: retrieval_tokens and processing_status tables are ready.")
	return nil
}

// Global mutexes for database operations
var (
	userMutexes         = make(map[string]*sync.Mutex)
	userMutexesLock     sync.Mutex
	resourceMutexes     = make(map[string]*sync.Mutex)
	resourceMutexesLock sync.Mutex
)

// getUserMutex returns a mutex for a specific user, creating it if it doesn't exist
func getUserMutex(userID string) *sync.Mutex {
	userMutexesLock.Lock()
	defer userMutexesLock.Unlock()

	if mutex, exists := userMutexes[userID]; exists {
		return mutex
	}

	mutex := &sync.Mutex{}
	userMutexes[userID] = mutex
	return mutex
}

// getResourceMutex returns a mutex for a specific resource, creating it if it doesn't exist
func getResourceMutex(userID, resourceID string) *sync.Mutex {
	key := fmt.Sprintf("%s_%s", userID, resourceID)

	resourceMutexesLock.Lock()
	defer resourceMutexesLock.Unlock()

	if mutex, exists := resourceMutexes[key]; exists {
		return mutex
	}

	mutex := &sync.Mutex{}
	resourceMutexes[key] = mutex
	return mutex
}

// storeRetrievalToken stores a new retrieval token or updates an existing one
func storeRetrievalToken(ctx context.Context, db *sql.DB, userID, platform, service, retrievalToken string) error {
	token := RetrievalToken{
		UserID:         userID,
		Platform:       platform,
		Service:        service,
		RetrievalToken: retrievalToken,
		RequiresUpdate: true,
	}
	return UpsertRetrievalToken(ctx, db, token)
}

// StoreGoogleDriveToken stores a Google Drive retrieval token
func StoreGoogleDriveToken(ctx context.Context, db *sql.DB, userID, retrievalToken string) error {
	token := RetrievalToken{
		UserID:         userID,
		Platform:       PlatformGoogle,
		Service:        ServiceDrive,
		RetrievalToken: retrievalToken,
		RequiresUpdate: true,
	}
	return UpsertRetrievalToken(ctx, db, token)
}

// StoreGoogleGmailToken stores a Google Gmail retrieval token
func StoreGoogleGmailToken(ctx context.Context, db *sql.DB, userID, retrievalToken string) error {
	token := RetrievalToken{
		UserID:         userID,
		Platform:       PlatformGoogle,
		Service:        ServiceGmail,
		RetrievalToken: retrievalToken,
		RequiresUpdate: true,
	}
	return UpsertRetrievalToken(ctx, db, token)
}

// StoreNotionToken stores a Notion retrieval token
func StoreNotionToken(ctx context.Context, db *sql.DB, userID, retrievalToken string) error {
	token := RetrievalToken{
		UserID:         userID,
		Platform:       PlatformNotion,
		Service:        ServiceNotion,
		RetrievalToken: retrievalToken,
		RequiresUpdate: true,
	}
	return UpsertRetrievalToken(ctx, db, token)
}

func StoreMicrosoftDriveToken(ctx context.Context, db *sql.DB, userID, retrievalToken string) error {
	token := RetrievalToken{
		UserID:         userID,
		Platform:       PlatformMicrosoft,
		Service:        ServiceMicrosoftDrive,
		RetrievalToken: retrievalToken,
		RequiresUpdate: true,
	}
	return UpsertRetrievalToken(ctx, db, token)
}

// UpsertRetrievalToken inserts or updates a retrieval token
func UpsertRetrievalToken(ctx context.Context, db *sql.DB, token RetrievalToken) error {
	mutex := getUserMutex(token.UserID)
	mutex.Lock()
	defer mutex.Unlock()

	now := time.Now()
	_, err := db.ExecContext(ctx, insertRetrievalTokenQuery,
		token.UserID,
		token.Platform,
		token.Service,
		token.RetrievalToken,
		now,
		now,
		token.RequiresUpdate,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert retrieval token: %w", err)
	}

	return nil
}

// DeleteRetrievalTokens deletes all retrieval tokens for a user and platform
func DeleteRetrievalTokens(ctx context.Context, db *sql.DB, userID, platform string) (int64, error) {
	mutex := getUserMutex(userID)
	mutex.Lock()
	defer mutex.Unlock()

	result, err := db.ExecContext(ctx, deleteRetrievalTokensQuery, userID, platform)
	if err != nil {
		return 0, fmt.Errorf("failed to delete retrieval tokens: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected, nil
}

// GetRetrievalTokens gets all retrieval tokens for a user
func GetRetrievalTokens(ctx context.Context, db *sql.DB, userID string) ([]RetrievalToken, error) {
	mutex := getUserMutex(userID)
	mutex.Lock()
	defer mutex.Unlock()

	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	rows, err := tx.QueryContext(ctx, getRetrievalTokensQuery, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query retrieval tokens: %w", err)
	}
	defer rows.Close()

	var tokens []RetrievalToken
	for rows.Next() {
		var token RetrievalToken
		token.UserID = userID
		if err := rows.Scan(&token.Platform, &token.Service, &token.RetrievalToken); err != nil {
			return nil, fmt.Errorf("failed to scan retrieval token: %w", err)
		}
		tokens = append(tokens, token)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating retrieval token rows: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return tokens, nil
}

// GetOutdatedTokens gets all tokens that need updating
func GetOutdatedTokens(ctx context.Context, db *sql.DB) ([]RetrievalToken, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	rows, err := tx.QueryContext(ctx, getOutdatedTokensQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query outdated tokens: %w", err)
	}
	defer rows.Close()

	var tokens []RetrievalToken
	for rows.Next() {
		var token RetrievalToken
		if err := rows.Scan(
			&token.UserID,
			&token.Platform,
			&token.Service,
			&token.RetrievalToken,
		); err != nil {
			return nil, fmt.Errorf("failed to scan outdated token: %w", err)
		}
		token.RequiresUpdate = true
		tokens = append(tokens, token)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating outdated token rows: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return tokens, nil
}

// UpsertProcessingStatus updates or inserts a processing status for a resource
func UpsertProcessingStatus(ctx context.Context, db *sql.DB, userID string, resourceID string, platform string, isProcessed bool) error {
	mutex := getResourceMutex(userID, resourceID)
	mutex.Lock()
	defer mutex.Unlock()

	now := time.Now()
	_, err := db.ExecContext(ctx, upsertProcessingStatusQuery,
		userID,
		resourceID,
		platform,
		isProcessed,
		false, // crawling_done is false initially or on upsert
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert processing status: %w", err)
	}

	return nil
}

// UpdateCrawlingDone updates the crawling_done status for a user
func UpdateCrawlingDone(ctx context.Context, db *sql.DB, userID string, platform string, done bool) error {
	mutex := getUserMutex(userID)
	mutex.Lock()
	defer mutex.Unlock()

	now := time.Now()
	_, err := db.ExecContext(ctx, updateCrawlingDoneQuery,
		userID,
		platform,
		done,
		now,
	)
	if err != nil {
		return fmt.Errorf("failed to update crawling done status: %w", err)
	}

	return nil
}

// GetProcessingStatus gets the processing status for a user
func GetProcessingStatus(ctx context.Context, db *sql.DB, userID string, platform string) (map[string]bool, bool, error) {
	mutex := getUserMutex(userID)
	mutex.Lock()
	defer mutex.Unlock()

	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	rows, err := tx.QueryContext(ctx, getProcessingStatusQuery, userID, platform)
	if err != nil {
		return nil, false, fmt.Errorf("failed to query processing status: %w", err)
	}
	defer rows.Close()

	processedFiles := make(map[string]bool)
	crawlingDone := false

	for rows.Next() {
		var resourceID string
		var isProcessed bool
		if err := rows.Scan(&resourceID, &isProcessed, &crawlingDone); err != nil {
			return nil, false, fmt.Errorf("failed to scan processing status: %w", err)
		}
		processedFiles[resourceID] = isProcessed
	}

	if err = rows.Err(); err != nil {
		return nil, false, fmt.Errorf("error iterating processing status rows: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return processedFiles, crawlingDone, nil
}

// DeleteProcessingStatus deletes all processing status entries for a user
func DeleteProcessingStatus(ctx context.Context, db *sql.DB, userID string, platform string) error {
	mutex := getUserMutex(userID)
	mutex.Lock()
	defer mutex.Unlock()

	_, err := db.ExecContext(ctx, deleteProcessingStatusQuery, userID, platform)
	if err != nil {
		return fmt.Errorf("failed to delete processing status: %w", err)
	}

	return nil
}

/***************************
** CHUNKID MAPPING CRUD **
***************************/

func generateShortKey(userID string, service string) string {
	timestamp := time.Now().UnixNano()
	randomPart := fmt.Sprintf("%x", timestamp)
	return fmt.Sprintf("%s_%s_%s", userID, service, randomPart)
}

// AddChunkMapping adds a new chunk mapping to the database with retries
func (s *crawlingServer) AddChunkMapping(ctx context.Context, userID string, serverName string, chunkID string, resourceID string, service string) (string, error) {
	mutex := getResourceMutex(userID, resourceID)
	mutex.Lock()
	defer mutex.Unlock()

	maxRetries := 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		shortKey, err := s.addChunkMappingInternal(ctx, userID, serverName, chunkID, resourceID, service)
		if err == nil {
			if err := s.verifyChunkMapping(ctx, userID, serverName, shortKey, resourceID); err != nil {
				log.Printf("Warning: Chunk mapping verification failed (attempt %d): %v", i+1, err)
				lastErr = err
				continue
			}
			return shortKey, nil
		}
		lastErr = err
		log.Printf("Warning: Failed to add chunk mapping (attempt %d): %v", i+1, err)
		time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
	}

	return "", fmt.Errorf("failed to add chunk mapping after %d attempts: %w", maxRetries, lastErr)
}

// addChunkMappingInternal handles the actual CouchDB operations
func (s *crawlingServer) addChunkMappingInternal(ctx context.Context, userID string, serverName string, chunkID string, resourceID string, service string) (string, error) {
	docID := fmt.Sprintf("%s_%s", userID, serverName)
	shortKey := generateShortKey(userID, service)

	now := time.Now().UTC().Format(time.RFC3339)
	newMapping := ChunkIDMapping{
		ShortKey:   shortKey,
		ChunkID:    chunkID,
		Service:    service,
		ResourceID: resourceID,
		CreatedAt:  now,
		LastUsed:   now,
	}

	doc := map[string]interface{}{
		"_id":           docID,
		"userID":        userID,
		"serverName":    serverName,
		"chunkMappings": []interface{}{newMapping},
		"createdAt":     now,
		"updatedAt":     now,
	}

	_, err := s.ChunkIDdb.Put(ctx, docID, doc)
	if err != nil {
		if strings.Contains(err.Error(), "conflict") {
			row := s.ChunkIDdb.Get(ctx, docID)
			if row.Err() != nil {
				return "", fmt.Errorf("failed to get existing document: %w", row.Err())
			}

			var existingDoc map[string]interface{}
			if err := row.ScanDoc(&existingDoc); err != nil {
				return "", fmt.Errorf("failed to scan existing document: %w", err)
			}

			originalMappings, _ := existingDoc["chunkMappings"].([]interface{})

			mappingMap := make(map[string]map[string]interface{})

			for _, mapping := range originalMappings {
				if m, ok := mapping.(map[string]interface{}); ok {
					if chunkID, ok := m["chunkId"].(string); ok {
						if resourceID, ok := m["resourceId"].(string); ok {
							key := fmt.Sprintf("%s:%s", chunkID, resourceID)
							mappingMap[key] = m
						}
					}
				}
			}

			key := fmt.Sprintf("%s:%s", chunkID, resourceID)
			if existingMapping, exists := mappingMap[key]; exists {
				existingMapping["lastUsed"] = now
				shortKey = existingMapping["shortKey"].(string)
			} else {
				newMappingMap := map[string]interface{}{
					"shortKey":   shortKey,
					"chunkId":    chunkID,
					"service":    service,
					"resourceId": resourceID,
					"createdAt":  now,
					"lastUsed":   now,
				}
				mappingMap[key] = newMappingMap
			}

			newMappings := make([]interface{}, 0, len(mappingMap))
			for _, m := range mappingMap {
				newMappings = append(newMappings, m)
			}

			existingDoc["chunkMappings"] = newMappings
			existingDoc["updatedAt"] = now

			_, err = s.ChunkIDdb.Put(ctx, docID, existingDoc)
			if err != nil {
				return "", fmt.Errorf("failed to update existing document: %w", err)
			}
		} else {
			return "", fmt.Errorf("failed to create document: %w", err)
		}
	}

	return shortKey, nil
}

// verifyChunkMapping checks if a mapping exists in the database
func (s *crawlingServer) verifyChunkMapping(ctx context.Context, userID string, serverName string, shortKey string, resourceID string) error {
	docID := fmt.Sprintf("%s_%s", userID, serverName)
	row := s.ChunkIDdb.Get(ctx, docID)
	if row.Err() != nil {
		return fmt.Errorf("failed to get document: %w", row.Err())
	}

	var doc map[string]interface{}
	if err := row.ScanDoc(&doc); err != nil {
		return fmt.Errorf("failed to scan document: %w", err)
	}

	mappings, ok := doc["chunkMappings"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid mappings format")
	}

	for _, mapping := range mappings {
		m, ok := mapping.(map[string]interface{})
		if !ok {
			continue
		}
		if m["shortKey"] == shortKey && m["resourceId"] == resourceID {
			return nil
		}
	}

	return fmt.Errorf("mapping not found after addition")
}

// DeleteChunkMappingsForFile deletes all chunk mappings for a specific resource with retries
func (s *crawlingServer) DeleteChunkMappingsForFile(ctx context.Context, userID string, serverName string, resourceID string) error {
	mutex := getResourceMutex(userID, resourceID)
	mutex.Lock()
	defer mutex.Unlock()

	maxRetries := 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		err := s.deleteChunkMappingsInternal(ctx, userID, serverName, resourceID)
		if err == nil {
			if err := s.verifyMappingsDeleted(ctx, userID, serverName, resourceID); err != nil {
				log.Printf("Warning: Chunk mappings deletion verification failed (attempt %d): %v", i+1, err)
				lastErr = err
				continue
			}
			return nil
		}
		lastErr = err
		log.Printf("Warning: Failed to delete chunk mappings (attempt %d): %v", i+1, err)
		time.Sleep(time.Duration(i+1) * 100 * time.Millisecond) // Exponential backoff
	}

	return fmt.Errorf("failed to delete chunk mappings after %d attempts: %w", maxRetries, lastErr)
}

// deleteChunkMappingsInternal handles the actual CouchDB operations for deletion
func (s *crawlingServer) deleteChunkMappingsInternal(ctx context.Context, userID string, serverName string, resourceID string) error {
	docID := fmt.Sprintf("%s_%s", userID, serverName)
	row := s.ChunkIDdb.Get(ctx, docID)
	if row.Err() != nil {
		if strings.Contains(row.Err().Error(), "not_found") {
			return nil
		}
		return fmt.Errorf("failed to get document: %w", row.Err())
	}

	var doc map[string]interface{}
	if err := row.ScanDoc(&doc); err != nil {
		return fmt.Errorf("failed to scan document: %w", err)
	}

	mappings, ok := doc["chunkMappings"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid mappings format in document")
	}

	var newMappings []interface{}
	for _, mapping := range mappings {
		m, ok := mapping.(map[string]interface{})
		if !ok {
			continue
		}
		if m["resourceId"] != resourceID {
			newMappings = append(newMappings, mapping)
		}
	}

	doc["chunkMappings"] = newMappings
	doc["updatedAt"] = time.Now().UTC().Format(time.RFC3339)

	_, err := s.ChunkIDdb.Put(ctx, docID, doc)
	if err != nil {
		return fmt.Errorf("failed to update document: %w", err)
	}

	return nil
}

// verifyMappingsDeleted checks if all mappings for a resource were deleted
func (s *crawlingServer) verifyMappingsDeleted(ctx context.Context, userID string, serverName string, resourceID string) error {
	docID := fmt.Sprintf("%s_%s", userID, serverName)
	row := s.ChunkIDdb.Get(ctx, docID)
	if row.Err() != nil {
		if strings.Contains(row.Err().Error(), "not_found") {
			return nil
		}
		return fmt.Errorf("failed to get document: %w", row.Err())
	}

	var doc map[string]interface{}
	if err := row.ScanDoc(&doc); err != nil {
		return fmt.Errorf("failed to scan document: %w", err)
	}

	mappings, ok := doc["chunkMappings"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid mappings format")
	}

	for _, mapping := range mappings {
		m, ok := mapping.(map[string]interface{})
		if !ok {
			continue
		}
		if m["resourceId"] == resourceID {
			return fmt.Errorf("found mapping for resource %s after deletion", resourceID)
		}
	}

	return nil
}

// DeleteChunkMappingsForPlatform deletes all chunk mappings for a user's platform from CouchDB
func (s *crawlingServer) DeleteChunkMappingsForPlatform(ctx context.Context, userID string, platform string) error {
	docID := fmt.Sprintf("%s_%s", userID, platform)
	row := s.ChunkIDdb.Get(ctx, docID)
	if row.Err() == nil {
		// Document exists, get its revision and delete it
		var doc map[string]interface{}
		if err := row.ScanDoc(&doc); err != nil {
			return fmt.Errorf("failed to scan CouchDB document: %v", err)
		}
		rev, ok := doc["_rev"].(string)
		if !ok {
			return fmt.Errorf("failed to get document revision from CouchDB")
		}
		_, err := s.ChunkIDdb.Delete(ctx, docID, rev)
		if err != nil {
			return fmt.Errorf("failed to delete chunk mappings from CouchDB: %v", err)
		}
	} else if !strings.Contains(row.Err().Error(), "not_found") {
		// Return error only if it's not a "not found" error
		return fmt.Errorf("failed to check CouchDB document: %v", row.Err())
	}
	return nil
}
