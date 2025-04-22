package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"

	pb "github.com/cc-0000/indeq/common/api"
	"golang.org/x/oauth2"
)

// createOAuthClient creates an OAuth client from an access token
func createGoogleOAuthClient(ctx context.Context, accessToken string) *http.Client {
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
	return oauth2.NewClient(ctx, tokenSource)
}

// GoogleCrawler crawls Google services (Drive, Gmail) based on provided scopes.
func (s *crawlingServer) GoogleCrawler(ctx context.Context, client *http.Client, userID string, scopes []string) error {
	scopeSet := make(map[string]struct{}, len(scopes))

	for _, scope := range scopes {
		scopeSet[scope] = struct{}{}
	}

	crawlers := map[string]func(context.Context, *http.Client, string) error{
		"https://www.googleapis.com/auth/drive.readonly": s.CrawlGoogleDrive,
		"https://www.googleapis.com/auth/gmail.readonly": s.CrawlGmail,
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(crawlers))
	results := make(chan string, len(crawlers))
	activeCrawlers := 0
	for scope, crawler := range crawlers {
		if _, ok := scopeSet[scope]; !ok {
			continue
		}
		activeCrawlers++
		wg.Add(1)
		go func(scope string, crawler func(context.Context, *http.Client, string) error) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			default:
				err := crawler(ctx, client, userID)
				if err != nil {
					errs <- fmt.Errorf("%s crawl failed: %w", scope, err)
					return
				}
				results <- scope
			}
		}(scope, crawler)
	}

	go func() {
		wg.Wait()
		close(errs)
		close(results)
	}()

	completedCrawlers := make(map[string]bool)
	for scope := range results {
		completedCrawlers[scope] = true
	}

	var errorList []error
	for err := range errs {
		if err != nil {
			log.Printf("Received error from crawler: %v", err)
			errorList = append(errorList, err)
		}
	}

	if activeCrawlers > 0 && len(completedCrawlers) == activeCrawlers {
		if err := s.sendCrawlDoneSignal(ctx, userID, "GOOGLE"); err != nil {
			log.Printf("Failed to send crawl done signal for Google services: %v", err)
		}
	}

	if len(errorList) > 0 {
		return fmt.Errorf("partial failure: %v", errorList)
	}
	return nil
}

// UpdateCrawlGoogle goes through specific service and return the new retrieval token
func (s *crawlingServer) UpdateCrawlGoogle(ctx context.Context, client *http.Client, service string, userID string, retrievalToken string) (string, error) {
	var newRetrievalToken string
	var err error

	switch service {
	case "GOOGLE_DRIVE":
		newRetrievalToken, err = s.UpdateCrawlGoogleDrive(ctx, client, userID, retrievalToken)
	case "GOOGLE_GMAIL":
		newRetrievalToken, err = s.UpdateCrawlGmail(ctx, client, userID, retrievalToken)
	default:
		return "", fmt.Errorf("unsupported service: %s", service)
	}

	if err != nil {
		return "", err
	}

	if err := s.sendCrawlDoneSignal(ctx, userID, "GOOGLE"); err != nil {
		log.Printf("Failed to send crawl done signal for Google %s update: %v", service, err)
	}

	return newRetrievalToken, nil
}

// GetChunksFromGoogle retrieves chunks from Google services based on metadata
func (s *crawlingServer) GetChunksFromGoogle(ctx context.Context, req *pb.GetChunksFromGoogleRequest) (*pb.GetChunksFromGoogleResponse, error) {
	accessToken, err := s.retrieveAccessToken(ctx, req.UserId, "GOOGLE")
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve access token: %w", err)
	}

	client := createGoogleOAuthClient(ctx, accessToken)

	resourceMap := make(map[string][]string)
	uniqueMetadata := make(map[string]*pb.Metadata)

	for _, metadata := range req.Metadatas {
		resourceMap[metadata.FileId] = append(resourceMap[metadata.FileId], metadata.ChunkId)
		if _, exists := uniqueMetadata[metadata.FileId]; !exists {
			uniqueMetadata[metadata.FileId] = metadata
		}
	}

	type chunkResult struct {
		resourceID string
		chunk      *pb.TextChunkMessage
		err        error
	}

	numWorkers, err := strconv.Atoi(os.Getenv("CRAWLING_GOOGLE_RETRIVAL_MAX_WORKERS"))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve the k value from the env variables: %w", err)
	}

	resultChan := make(chan chunkResult, len(uniqueMetadata))
	var wg sync.WaitGroup

	uniqueMetadataList := make([]*pb.Metadata, 0, len(uniqueMetadata))
	for _, metadata := range uniqueMetadata {
		uniqueMetadataList = append(uniqueMetadataList, metadata)
	}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(start int) {
			defer wg.Done()
			for j := start; j < len(uniqueMetadataList); j += numWorkers {
				metadata := uniqueMetadataList[j]
				internalMetadata := Metadata{
					DateCreated:      metadata.DateCreated.AsTime(),
					DateLastModified: metadata.DateLastModified.AsTime(),
					UserID:           metadata.UserId,
					ResourceID:       metadata.FileId,
					ResourceType:     metadata.ResourceType,
					FileURL:          metadata.FileUrl,
					Title:            metadata.Title,
					ChunkID:          metadata.ChunkId,
					FilePath:         metadata.FilePath,
					Platform:         "GOOGLE",
					Service:          metadata.Service,
				}

				chunks, err := RetrieveGoogleCrawler(ctx, client, internalMetadata, resourceMap[metadata.FileId])
				if err != nil {
					resultChan <- chunkResult{
						resourceID: metadata.FileId,
						err:        fmt.Errorf("error retrieving chunk for %s: %w", internalMetadata.FilePath, err),
					}
					continue
				}
				for _, chunk := range chunks {
					protoChunk := &pb.TextChunkMessage{
						Metadata: s.convertToProtoMetadata(chunk.Metadata),
						Content:  chunk.Content,
					}
					resultChan <- chunkResult{resourceID: metadata.FileId, chunk: protoChunk}
				}
			}
		}(i)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var chunks []*pb.TextChunkMessage
	var errs []error

	for result := range resultChan {
		if result.err != nil {
			errs = append(errs, result.err)
			log.Printf("Warning: %v", result.err)
			continue
		}
		if result.chunk != nil {
			chunks = append(chunks, result.chunk)
		}
	}

	if len(errs) > 0 {
		log.Printf("Some chunks failed to retrieve: %v", errs)
	}

	return &pb.GetChunksFromGoogleResponse{
		NumChunks: int64(len(chunks)),
		Chunks:    chunks,
	}, nil
}

func RetrieveGoogleCrawler(ctx context.Context, client *http.Client, metadata Metadata, chunkIDs []string) ([]TextChunkMessage, error) {
	if metadata.Service == "GOOGLE_DRIVE" {
		return RetrieveFromDrive(ctx, client, metadata, chunkIDs)
	}
	if metadata.Service == "GOOGLE_GMAIL" {
		return RetrieveFromGmail(ctx, client, metadata)
	}

	return nil, fmt.Errorf("unsupported service: %s", metadata.Service)
}
