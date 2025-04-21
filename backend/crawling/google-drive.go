package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	pb "github.com/cc-0000/indeq/common/api"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// MimeHandlers maps MIME types to their processing functions
var mimeHandlers = map[string]func(context.Context, *http.Client, File) (File, error){
	"application/vnd.google-apps.document":     ProcessGoogleDoc,
	"application/vnd.google-apps.presentation": ProcessGoogleSlides,
}

var (
	folderCache    sync.Map
	folderCacheTTL = 5 * time.Minute
)

type cachedFolder struct {
	hierarchy string
	timestamp time.Time
}

// CrawlGoogleDrive retrieves and processes files from Google Drive
func (s *crawlingServer) CrawlGoogleDrive(ctx context.Context, client *http.Client, userID string) error {
	filelist, err := GetGoogleDriveList(ctx, client, userID)
	if err != nil {
		return fmt.Errorf("error retrieving Google Drive file list: %w", err)
	}
	err = s.ProcessAllGoogleDriveFiles(ctx, client, filelist)
	if err != nil {
		return fmt.Errorf("error processing Google Drive files: %w", err)
	}
	retrievalToken, err := GetStartPageToken(ctx, client)
	if err != nil {
		return fmt.Errorf("error getting start page token: %w", err)
	}
	if err := StoreGoogleDriveToken(ctx, s.db, userID, retrievalToken); err != nil {
		return fmt.Errorf("failed to store change token: %w", err)
	}
	return nil
}

// UpdateCrawlGoogleDrive retrieves and processes changes in Google Drive
func (s *crawlingServer) UpdateCrawlGoogleDrive(ctx context.Context, client *http.Client, userID string, changeToken string) (string, error) {
	filelist, newChangeToken, err := GetGoogleDriveChanges(ctx, client, changeToken, userID)
	if err != nil {
		return "", fmt.Errorf("error retrieving Google Drive changes: %w", err)
	}
	if len(filelist.Files) == 0 {
		return changeToken, nil
	}
	var filePaths []string
	for _, file := range filelist.Files {
		if len(file.File) > 0 {
			filePaths = append(filePaths, file.File[0].Metadata.FilePath)
			resourceID := file.File[0].Metadata.ResourceID
			if s.isFileProcessed(userID, resourceID, "GOOGLE") {
				if err := UpsertProcessingStatus(ctx, s.db, userID, resourceID, "GOOGLE", false); err != nil {
					log.Printf("Warning: failed to reset tracking status for file %s: %v", resourceID, err)
				}
			}
		}
	}

	if len(filePaths) > 0 {
		_, err = s.vectorService.DeleteFiles(ctx, &pb.VectorFileDeleteRequest{
			UserId:    userID,
			Platform:  pb.Platform_PLATFORM_GOOGLE,
			Files:     filePaths,
			Exclusive: false,
		})
		if err != nil {
			log.Printf("Warning: failed to delete old file versions: %v", err)
		}
	}

	err = s.ProcessAllGoogleDriveFiles(ctx, client, filelist)
	if err != nil {
		return "", fmt.Errorf("error processing Google Drive files: %w", err)
	}
	return newChangeToken, nil
}

// createGoogleFileMetadata creates metadata for a Google Drive file
func createGoogleFileMetadata(srv *drive.Service, userID string, file *drive.File) (Metadata, error) {
	createdTime := parseTime(file.CreatedTime)
	modifiedTime := parseTime(file.ModifiedTime)

	hierarchy := "/"
	if len(file.Parents) > 0 {
		h, err := getFolderHierarchy(srv, file.Parents)
		if err == nil {
			hierarchy = h
		}
	}

	filePath := buildFilePath(hierarchy, file.Name)

	return Metadata{
		DateCreated:      createdTime,
		DateLastModified: modifiedTime,
		UserID:           userID,
		ResourceID:       file.Id,
		Title:            file.Name,
		ResourceType:     file.MimeType,
		FileURL:          file.WebViewLink,
		FilePath:         filePath,
		Platform:         "GOOGLE",
		Service:          "GOOGLE_DRIVE",
	}, nil
}

// isValidGoogleFile checks if the file is a supported Google file type
func isValidGoogleFile(mimeType string) bool {
	return mimeType == "application/vnd.google-apps.document" ||
		mimeType == "application/vnd.google-apps.presentation"
}

// GetGoogleDriveList retrieves the list of files from Google Drive
func GetGoogleDriveList(ctx context.Context, client *http.Client, userID string) (ListofFiles, error) {
	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return ListofFiles{}, fmt.Errorf("failed to create Google Drive service: %w", err)
	}
	var fileList ListofFiles
	pageToken := ""
	const pageSize = 1000

	for {
		if err := rateLimiter.Wait(ctx, "GOOGLE_DRIVE", userID); err != nil {
			return ListofFiles{}, fmt.Errorf("rate limit wait failed: %w", err)
		}

		listCall := srv.Files.List().
			PageSize(pageSize).
			Fields("nextPageToken, files(id, name, mimeType, createdTime, modifiedTime, webViewLink, parents, trashed)")
		if pageToken != "" {
			listCall = listCall.PageToken(pageToken)
		}

		res, err := listCall.Do()
		if err != nil {
			return ListofFiles{}, fmt.Errorf("failed to list files: %w", err)
		}

		for _, driveFile := range res.Files {
			if !isValidGoogleFile(driveFile.MimeType) {
				continue
			}

			metadata, err := createGoogleFileMetadata(srv, userID, driveFile)
			if err != nil {
				log.Printf("Warning: failed to create metadata for file %s: %v", driveFile.Id, err)
				continue
			}

			fileList.Files = append(fileList.Files, File{
				File: []TextChunkMessage{
					{
						Metadata: metadata,
						Content:  "",
					},
				},
			})
		}

		if res.NextPageToken == "" {
			break
		}
		pageToken = res.NextPageToken
	}

	return fileList, nil
}

// GetGoogleDriveChanges retrieves changes in Google Drive files
func GetGoogleDriveChanges(ctx context.Context, client *http.Client, retrievalToken string, userID string) (ListofFiles, string, error) {
	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return ListofFiles{}, "", fmt.Errorf("failed to create Google Drive service: %w", err)
	}

	var fileList ListofFiles
	pageToken := retrievalToken
	hasChanges := false

	for {
		changesCall := srv.Changes.List(pageToken).
			Fields("nextPageToken, newStartPageToken, changes(fileId, file(id, name, mimeType, createdTime, modifiedTime, webViewLink, trashed, parents))")

		res, err := changesCall.Do()
		if err != nil {
			return ListofFiles{}, "", fmt.Errorf("failed to list changes: %w", err)
		}

		for _, change := range res.Changes {
			if change.File == nil || !isValidGoogleFile(change.File.MimeType) {
				continue
			}

			if change.Removed {
				fileList.Files = append(fileList.Files, createRemovedFileEntry(userID, change.FileId))
				hasChanges = true
				continue
			}

			metadata, err := createGoogleFileMetadata(srv, userID, change.File)
			if err != nil {
				log.Printf("Warning: failed to create metadata for changed file %s: %v", change.FileId, err)
				continue
			}

			fileList.Files = append(fileList.Files, File{
				File: []TextChunkMessage{
					{
						Metadata: metadata,
						Content:  "",
					},
				},
			})
			hasChanges = true
		}

		if res.NextPageToken == "" {
			newToken := res.NewStartPageToken
			if !hasChanges {
				return ListofFiles{}, retrievalToken, nil
			}
			return fileList, newToken, nil
		}
		pageToken = res.NextPageToken
	}
}

// Helper function to parse time with fallback
func parseTime(timeStr string) time.Time {
	parsedTime, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return time.Now()
	}
	return parsedTime
}

// Helper function to create removed file entry
func createRemovedFileEntry(userID, fileID string) File {
	return File{
		File: []TextChunkMessage{
			{
				Metadata: Metadata{
					UserID:     userID,
					ResourceID: fileID,
					Platform:   "GOOGLE",
					Service:    "GOOGLE_DRIVE",
				},
			},
		},
	}
}

// ProcessAllGoogleDriveFiles processes all Google Drive files concurrently by MIME type
func (s *crawlingServer) ProcessAllGoogleDriveFiles(ctx context.Context, client *http.Client, fileList ListofFiles) error {
	type result struct {
		index int
		file  File
		err   error
	}

	chunkBatchSize := 50
	chunkCh := make(chan TextChunkMessage, chunkBatchSize)
	var chunkWg sync.WaitGroup

	chunkWg.Add(1)
	go func() {
		defer chunkWg.Done()
		batch := make([]TextChunkMessage, 0, chunkBatchSize)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		processBatch := func() {
			if len(batch) > 0 {
				if err := s.sendChunkBatchToVector(ctx, batch); err != nil {
					log.Printf("Error sending chunk batch: %v", err)
				}
				batch = batch[:0]
			}
		}

		for {
			select {
			case chunk, ok := <-chunkCh:
				if !ok {
					processBatch()
					return
				}
				batch = append(batch, chunk)
				if len(batch) >= chunkBatchSize {
					processBatch()
				}
			case <-ticker.C:
				processBatch()
			case <-ctx.Done():
				processBatch()
				return
			}
		}
	}()

	type workItem struct {
		index int
		file  File
	}
	workItemsByType := make(map[string][]workItem)
	resultCh := make(chan result, len(fileList.Files))

	for i, file := range fileList.Files {
		if len(file.File) == 0 || file.File[0].Metadata.ResourceType == "" {
			resultCh <- result{index: i, file: file}
			continue
		}
		mimeType := file.File[0].Metadata.ResourceType
		workItemsByType[mimeType] = append(workItemsByType[mimeType], workItem{
			index: i,
			file:  file,
		})
	}

	var wg sync.WaitGroup
	for mimeType, items := range workItemsByType {
		numWorkers := 5
		var err error
		switch mimeType {
		case "application/vnd.google-apps.document":
			numWorkers, err = strconv.Atoi(os.Getenv("CRAWLING_GOOGLEDOCS_MAX_WORKERS"))
			if err != nil {
				fmt.Printf("Warning: failed to retrieve the k value from the env variables: %v", err)
			}

		case "application/vnd.google-apps.presentation":
			numWorkers, err = strconv.Atoi(os.Getenv("CRAWLING_GOOGLESLIDES_MAX_WORKERS"))
			if err != nil {
				fmt.Printf("Warning: failed to retrieve the k value from the env variables: %v", err)
			}
		}

		// Create worker pool for this MIME type
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(mimeType string, workerID int, items []workItem) {
				defer wg.Done()
				handler, ok := mimeHandlers[mimeType]
				if !ok {
					for _, item := range items {
						resultCh <- result{index: item.index, file: item.file}
					}
					return
				}

				for j := workerID; j < len(items); j += numWorkers {
					item := items[j]
					originalFilePath := item.file.File[0].Metadata.FilePath
					originalUserID := item.file.File[0].Metadata.UserID
					originalResourceID := item.file.File[0].Metadata.ResourceID
					if s.isFileProcessed(originalUserID, originalResourceID, "GOOGLE") {
						resultCh <- result{index: item.index, file: item.file}
						continue
					}

					processedFile, err := handler(ctx, client, item.file)
					if err != nil {
						resultCh <- result{index: item.index, err: fmt.Errorf("error processing file %s: %w", originalResourceID, err)}
						continue
					}

					if len(processedFile.File) > 0 {
						for _, chunk := range processedFile.File {
							select {
							case chunkCh <- chunk:
							case <-ctx.Done():
								return
							}
						}

						if err := s.sendFileDoneSignal(ctx, originalUserID, originalFilePath, "GOOGLE"); err != nil {
							log.Printf("Error sending file done signal for %s: %v", originalFilePath, err)
						}
					} else {
						log.Printf("No chunks generated for file: %s", originalFilePath)
					}

					resultCh <- result{index: item.index, file: processedFile}
				}
			}(mimeType, i, items)
		}
	}

	go func() {
		wg.Wait()
		close(resultCh)
		close(chunkCh)
	}()

	processedFiles := make([]File, len(fileList.Files))
	var errs []error
	var userID string
	var successfullyProcessed int

	for res := range resultCh {
		if res.err != nil {
			errs = append(errs, res.err)
			continue
		}
		processedFiles[res.index] = res.file
		if len(res.file.File) > 0 {
			successfullyProcessed++
			if userID == "" {
				userID = res.file.File[0].Metadata.UserID
			}
		}
	}

	chunkWg.Wait()

	if len(errs) > 0 {
		return fmt.Errorf("some files failed to process: %v", errs)
	}
	if userID != "" {
		log.Printf("Drive crawling complete for user %s", userID)
	} else {
		log.Print("No valid files found in Drive crawl")
	}
	return nil
}

// GetStartPageToken retrieves the start page token for Google Drive changes
func GetStartPageToken(ctx context.Context, client *http.Client) (string, error) {
	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return "", fmt.Errorf("failed to create Google Drive service: %w", err)
	}

	changeToken, err := srv.Changes.GetStartPageToken().Do()
	if err != nil {
		return "", fmt.Errorf("unable to retrieve start page token: %v", err)
	}

	return changeToken.StartPageToken, nil
}

// RetrieveFromDrive retrieves content based on file type
func RetrieveFromDrive(ctx context.Context, client *http.Client, metadata Metadata) (TextChunkMessage, error) {
	switch metadata.ResourceType {
	case "application/vnd.google-apps.document":
		return RetrieveGoogleDoc(ctx, client, metadata)
	case "application/vnd.google-apps.presentation":
		return RetrieveGoogleSlides(ctx, client, metadata)
	default:
		return TextChunkMessage{}, fmt.Errorf("unsupported resource type: %s", metadata.ResourceType)
	}
}

func buildFilePath(hierarchy, fileName string) string {
	filePath := hierarchy
	if hierarchy != "/" {
		filePath += "/"
	}
	return filePath + fileName
}

// getFolderHierarchy resolves the folder hierarchy for a given file
func getFolderHierarchy(srv *drive.Service, parentIDs []string) (string, error) {
	if len(parentIDs) == 0 {
		return "/", nil
	}

	cacheKey := strings.Join(parentIDs, ",")

	if cached, ok := folderCache.Load(cacheKey); ok {
		entry := cached.(cachedFolder)
		if time.Since(entry.timestamp) < folderCacheTTL {
			return entry.hierarchy, nil
		}
		folderCache.Delete(cacheKey)
	}

	pathParts := make([]string, 0, len(parentIDs))
	currentID := parentIDs[0]

	for currentID != "" {
		if err := rateLimiter.Wait(context.Background(), "GOOGLE_DRIVE", "system"); err != nil {
			return "/", fmt.Errorf("rate limit error: %w", err)
		}

		folder, err := srv.Files.Get(currentID).
			Fields("name, parents").
			SupportsAllDrives(true).
			Do()
		if err != nil {
			return "/", fmt.Errorf("failed to get folder %s: %w", currentID, err)
		}

		pathParts = append([]string{folder.Name}, pathParts...)
		if len(folder.Parents) == 0 {
			break
		}
		currentID = folder.Parents[0]
	}

	result := "/" + strings.Join(pathParts, "/")

	folderCache.Store(cacheKey, cachedFolder{
		hierarchy: result,
		timestamp: time.Now(),
	})

	return result, nil
}

// Add new helper function for batch vector processing
func (s *crawlingServer) sendChunkBatchToVector(ctx context.Context, chunks []TextChunkMessage) error {
	if len(chunks) == 0 {
		return nil
	}

	const maxParallelBatches = 5
	batchSize := (len(chunks) + maxParallelBatches - 1) / maxParallelBatches
	var wg sync.WaitGroup
	errCh := make(chan error, maxParallelBatches)

	for i := 0; i < len(chunks); i += batchSize {
		end := i + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}

		wg.Add(1)
		go func(batch []TextChunkMessage) {
			defer wg.Done()
			for _, chunk := range batch {
				if err := s.sendChunkToVector(ctx, chunk); err != nil {
					errCh <- fmt.Errorf("error sending chunk to vector service: %w", err)
					return
				}
			}
		}(chunks[i:end])
	}

	go func() {
		wg.Wait()
		close(errCh)
	}()

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("batch processing errors: %v", errs)
	}

	return nil
}
