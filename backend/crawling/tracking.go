package main

import (
	"context"
	"log"
)

// Check if a file is processed
func (s *crawlingServer) isFileProcessed(userID, resourceID string, platform string) bool {
	ctx := context.Background()
	processedFiles, _, err := GetProcessingStatus(ctx, s.db, userID, platform)
	if err != nil {
		log.Printf("Error checking file processed status: %v", err)
		return false
	}

	return processedFiles[resourceID]
}

// Mark a file as processed
func (s *crawlingServer) markFileProcessed(userID, resourceID string, platform string) {
	ctx := context.Background()
	err := UpsertProcessingStatus(ctx, s.db, userID, resourceID, platform, true)
	if err != nil {
		log.Printf("Error marking file as processed: %v", err)
	}
}

// Mark crawling as complete
func (s *crawlingServer) markCrawlingComplete(userID string, platform string) {
	ctx := context.Background()
	err := UpdateCrawlingDone(ctx, s.db, userID, platform, true)
	if err != nil {
		log.Printf("Error marking crawling as complete: %v", err)
	}
}
