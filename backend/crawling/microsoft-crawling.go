package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"baliance.com/gooxml/document"
	"baliance.com/gooxml/presentation"
	pb "github.com/cc-0000/indeq/common/api"
	"golang.org/x/oauth2"
)

func createMicrosoftOAuthClient(ctx context.Context, accessToken string) *http.Client {
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
	return oauth2.NewClient(ctx, tokenSource)
}

func (s *crawlingServer) MicrosoftCrawler(ctx context.Context, client *http.Client, userID string) error {
	files, delta, err := s.GetMicrosoftDriveFiles(ctx, client, userID, "")
	if err != nil {
		return fmt.Errorf("failed to get Microsoft Drive files: %w", err)
	}
	err = s.ProcessMicrosoftDriveFiles(ctx, client, userID, files)
	if err != nil {
		return fmt.Errorf("failed to process Microsoft Drive files: %w", err)
	}
	if err := StoreMicrosoftDriveToken(ctx, s.db, userID, delta); err != nil {
		return fmt.Errorf("failed to store change token: %w", err)
	}
	if err := s.sendCrawlDoneSignal(ctx, userID, "MICROSOFT"); err != nil {
		log.Printf("Failed to send crawl done signal for Microsoft services: %v", err)
	}
	return nil
}

func (s *crawlingServer) UpdateCrawlMicrosoft(ctx context.Context, client *http.Client, userID, retrievalToken string) (string, error) {
	files, delta, err := s.GetMicrosoftDriveFiles(ctx, client, userID, retrievalToken)
	if err != nil {
		return "", fmt.Errorf("failed to get Microsoft Drive files: %w", err)
	}
	err = s.ProcessMicrosoftDriveFiles(ctx, client, userID, files)
	if err != nil {
		return "", fmt.Errorf("failed to process Microsoft Drive files: %w", err)
	}
	if err := s.sendCrawlDoneSignal(ctx, userID, "MICROSOFT"); err != nil {
		log.Printf("Failed to send crawl done signal for Microsoft services: %v", err)
	}
	return delta, nil
}

func (s *crawlingServer) GetMicrosoftDriveFiles(ctx context.Context, client *http.Client, userID string, retrievalToken string) (ListofFiles, string, error) {
	var fileList ListofFiles
	pageToken := ""
	delta := ""
	url := ""
	for {
		if err := rateLimiter.Wait(ctx, "MICROSOFT_DRIVE", userID); err != nil {
			return ListofFiles{}, "", fmt.Errorf("rate limit wait failed: %w", err)
		}

		switch {
		case pageToken != "":
			url = pageToken
		case retrievalToken != "" && delta == "":
			url = retrievalToken
		default:
			url = "https://graph.microsoft.com/v1.0/me/drive/root/delta?$top=1000&$select=id,name,webUrl,createdDateTime,lastModifiedDateTime,file,parentReference"
		}

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return ListofFiles{}, "", fmt.Errorf("failed to create request: %w", err)
		}

		token, err := client.Transport.(*oauth2.Transport).Source.Token()
		if err != nil {
			return ListofFiles{}, "", fmt.Errorf("failed to get access token: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return ListofFiles{}, "", fmt.Errorf("failed to list files: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return ListofFiles{}, "", fmt.Errorf("failed to read response body: %w", err)
		}

		var driveResponse struct {
			Value []struct {
				ID       string    `json:"id"`
				Name     string    `json:"name"`
				WebURL   string    `json:"webUrl"`
				Created  time.Time `json:"createdDateTime"`
				Modified time.Time `json:"lastModifiedDateTime"`
				File     struct {
					MimeType string `json:"mimeType"`
				} `json:"file"`
				ParentRef struct {
					Path string `json:"path"`
				} `json:"parentReference"`
			} `json:"value"`
			NextLink  string `json:"@odata.nextLink"`
			DeltaLink string `json:"@odata.deltaLink"`
		}

		if err := json.Unmarshal(body, &driveResponse); err != nil {
			return ListofFiles{}, "", fmt.Errorf("failed to decode response: %w", err)
		}

		for _, driveFile := range driveResponse.Value {
			if !isValidMicrosoftFile(driveFile.File.MimeType) {
				continue
			}

			metadata := Metadata{
				DateCreated:      driveFile.Created,
				DateLastModified: driveFile.Modified,
				UserID:           userID,
				ResourceID:       driveFile.ID,
				Title:            driveFile.Name,
				ResourceType:     driveFile.File.MimeType,
				FileURL:          driveFile.WebURL,
				FilePath:         buildFilePath(driveFile.ParentRef.Path, driveFile.Name),
				Platform:         "MICROSOFT",
				Service:          "MICROSOFT_DRIVE",
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

		if driveResponse.NextLink == "" {
			delta = driveResponse.DeltaLink
			break
		}
		pageToken = driveResponse.NextLink
	}

	return fileList, delta, nil
}

func isValidMicrosoftFile(mimeType string) bool {
	validTypes := map[string]bool{
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   true,
		"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
		"application/msword":            true,
		"application/vnd.ms-powerpoint": true,
	}
	return validTypes[mimeType]
}

func (s *crawlingServer) ProcessMicrosoftDriveFiles(ctx context.Context, client *http.Client, userID string, files ListofFiles) error {
	token, err := client.Transport.(*oauth2.Transport).Source.Token()
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "microsoft-files-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	type result struct {
		index int
		file  File
		err   error
	}

	resultChan := make(chan result, len(files.Files))
	var wg sync.WaitGroup

	for i := range files.Files {
		wg.Add(1)
		go func(index int, file File) {
			defer wg.Done()

			if len(file.File) == 0 {
				resultChan <- result{index: index, file: file}
				return
			}

			localPath := filepath.Join(tempDir, filepath.Base(file.File[0].Metadata.FilePath))
			err := downloadFile(file.File[0].Metadata.ResourceID, token.AccessToken, localPath)
			if err != nil {
				resultChan <- result{index: index, err: fmt.Errorf("error downloading file: %w", err)}
				return
			}

			var text string
			mimeType := file.File[0].Metadata.ResourceType
			switch mimeType {
			case "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "application/msword":
				text, err = extractDocxText(localPath)
			case "application/vnd.openxmlformats-officedocument.presentationml.presentation", "application/vnd.ms-powerpoint":
				text, err = extractPptxText(localPath)
			default:
				resultChan <- result{index: index, file: file}
				return
			}
			if err != nil {
				log.Printf("Error extracting text from file: %s", err)
				resultChan <- result{index: index, file: file}
				return
			}

			words := strings.Fields(text)
			totalWords := len(words)
			if totalWords == 0 {
				resultChan <- result{index: index, file: file}
				return
			}

			chunkSize := 400
			overlap := 80
			if totalWords < chunkSize {
				chunkSize = totalWords
				overlap = 0
			}

			var fileChunks []TextChunkMessage

			for start := 0; start < totalWords; start += chunkSize - overlap {
				end := start + chunkSize
				if end > totalWords {
					end = totalWords
				}

				if start > 0 && end-start < overlap {
					continue
				}

				chunkWords := words[start:end]
				chunkText := strings.Join(chunkWords, " ")

				metadata := file.File[0].Metadata
				metadata.ChunkID = fmt.Sprintf("startoffset:%d-endoffset:%d", start, end-1)

				fileChunks = append(fileChunks, TextChunkMessage{
					Metadata: metadata,
					Content:  chunkText,
				})
				if err := s.sendChunkToVector(ctx, fileChunks[len(fileChunks)-1]); err != nil {
					log.Printf("Error sending chunk to vector: %s", err)
					continue
				}
			}

			processedFile := File{
				File: fileChunks,
			}

			if err := s.sendFileDoneSignal(ctx, file.File[0].Metadata.UserID, file.File[0].Metadata.FilePath, "MICROSOFT"); err != nil {
				log.Printf("Error sending file done signal for %s: %v", file.File[0].Metadata.FilePath, err)
			}

			resultChan <- result{index: index, file: processedFile}
		}(i, files.Files[i])
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	processedFiles := make([]File, len(files.Files))
	var errs []error
	for res := range resultChan {
		if res.err != nil {
			errs = append(errs, res.err)
			continue
		}
		processedFiles[res.index] = res.file
	}

	if len(errs) > 0 {
		return fmt.Errorf("some files failed to process: %v", errs)
	}

	return nil
}

func downloadFile(itemID, accessToken, outputPath string) error {
	url := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/drive/items/%s/content", itemID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func extractDocxText(filePath string) (string, error) {
	doc, err := document.Open(filePath)
	if err != nil {
		return "", err
	}
	var text string
	for _, para := range doc.Paragraphs() {
		for _, run := range para.Runs() {
			text += run.Text()
		}
		text += "\n"
	}
	return text, nil
}

func extractPptxText(filePath string) (string, error) {
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0700)
	if err != nil {
		return "", fmt.Errorf("failed to open null device: %w", err)
	}
	defer devNull.Close()

	originalLogger := log.Default().Writer()
	log.Default().SetOutput(devNull)
	defer log.Default().SetOutput(originalLogger)

	pres, err := presentation.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open PowerPoint file: %w", err)
	}

	var text string
	for _, slide := range pres.Slides() {
		for _, placeholder := range slide.PlaceHolders() {
			shape := placeholder.X()
			if shape.TxBody != nil {
				for _, para := range shape.TxBody.P {
					for _, run := range para.EG_TextRun {
						if run.R != nil {
							text += run.R.T
						}
					}
					text += "\n"
				}
			}
		}
		text += "\n"
	}

	if text == "" {
		return "", fmt.Errorf("no text content found in PowerPoint file")
	}

	return text, nil
}

func RetrieveMicrosoftCrawler(ctx context.Context, client *http.Client, metadata Metadata) (TextChunkMessage, error) {
	if metadata.Service == "MICROSOFT_DRIVE" {
		return RetrieveFromMicrosoftDrive(ctx, client, metadata)
	}
	return TextChunkMessage{}, fmt.Errorf("unsupported service: %s", metadata.Service)
}

func RetrieveFromMicrosoftDrive(ctx context.Context, client *http.Client, metadata Metadata) (TextChunkMessage, error) {
	switch metadata.ResourceType {
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "application/msword":
		return RetrieveFromDocx(ctx, client, metadata)
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation", "application/vnd.ms-powerpoint":
		return RetrieveFromPptx(ctx, client, metadata)
	default:
		return TextChunkMessage{}, fmt.Errorf("unsupported resource type: %s", metadata.ResourceType)
	}
}

func RetrieveFromDocx(ctx context.Context, client *http.Client, metadata Metadata) (TextChunkMessage, error) {
	chunkId := metadata.ChunkID

	token, err := client.Transport.(*oauth2.Transport).Source.Token()
	if err != nil {
		return TextChunkMessage{}, fmt.Errorf("failed to get access token: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "microsoft-retrieval-*")
	if err != nil {
		return TextChunkMessage{}, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	localPath := filepath.Join(tempDir, filepath.Base(metadata.FilePath))
	err = downloadFile(metadata.ResourceID, token.AccessToken, localPath)
	if err != nil {
		return TextChunkMessage{}, fmt.Errorf("failed to download file: %w", err)
	}

	doc, err := document.Open(localPath)
	if err != nil {
		return TextChunkMessage{}, fmt.Errorf("failed to open document: %w", err)
	}

	var allText string
	for _, para := range doc.Paragraphs() {
		for _, run := range para.Runs() {
			allText += run.Text()
		}
		allText += "\n"
	}

	if chunkId == "" {
		return TextChunkMessage{Metadata: metadata, Content: allText}, nil
	}

	var startOffset, endOffset int
	_, err = fmt.Sscanf(chunkId, "startoffset:%d-endoffset:%d", &startOffset, &endOffset)
	if err != nil {
		return TextChunkMessage{}, fmt.Errorf("invalid chunk ID format: %w", err)
	}

	words := strings.Fields(allText)

	if startOffset < 0 || endOffset >= len(words) || startOffset > endOffset {
		return TextChunkMessage{}, fmt.Errorf("invalid offset range: start=%d, end=%d, total words=%d",
			startOffset, endOffset, len(words))
	}

	chunkWords := words[startOffset : endOffset+1]
	chunkText := strings.Join(chunkWords, " ")

	return TextChunkMessage{Metadata: metadata, Content: chunkText}, nil
}

func RetrieveFromPptx(ctx context.Context, client *http.Client, metadata Metadata) (TextChunkMessage, error) {
	chunkId := metadata.ChunkID

	token, err := client.Transport.(*oauth2.Transport).Source.Token()
	if err != nil {
		return TextChunkMessage{}, fmt.Errorf("failed to get access token: %w", err)
	}

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "microsoft-retrieval-*")
	if err != nil {
		return TextChunkMessage{}, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	localPath := filepath.Join(tempDir, filepath.Base(metadata.FilePath))
	// Download file
	err = downloadFile(metadata.ResourceID, token.AccessToken, localPath)
	if err != nil {
		return TextChunkMessage{}, fmt.Errorf("failed to download file: %w", err)
	}

	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0700)
	if err != nil {
		return TextChunkMessage{}, fmt.Errorf("failed to open null device: %w", err)
	}
	defer devNull.Close()

	originalLogger := log.Default().Writer()
	log.Default().SetOutput(devNull)
	defer log.Default().SetOutput(originalLogger)

	pres, err := presentation.Open(localPath)
	if err != nil {
		return TextChunkMessage{}, fmt.Errorf("failed to open PowerPoint file: %w", err)
	}

	var allText string
	for _, slide := range pres.Slides() {
		for _, placeholder := range slide.PlaceHolders() {
			shape := placeholder.X()
			if shape.TxBody != nil {
				for _, para := range shape.TxBody.P {
					for _, run := range para.EG_TextRun {
						if run.R != nil {
							allText += run.R.T
						}
					}
					allText += "\n"
				}
			}
		}
		allText += "\n"
	}

	if allText == "" {
		return TextChunkMessage{}, fmt.Errorf("no text content found in PowerPoint file")
	}

	if chunkId == "" {
		return TextChunkMessage{Metadata: metadata, Content: allText}, nil
	}

	var startOffset, endOffset int
	_, err = fmt.Sscanf(chunkId, "startoffset:%d-endoffset:%d", &startOffset, &endOffset)
	if err != nil {
		return TextChunkMessage{}, fmt.Errorf("invalid chunk ID format: %w", err)
	}

	words := strings.Fields(allText)

	if startOffset < 0 || endOffset >= len(words) || startOffset > endOffset {
		return TextChunkMessage{}, fmt.Errorf("invalid offset range: start=%d, end=%d, total words=%d",
			startOffset, endOffset, len(words))
	}

	chunkWords := words[startOffset : endOffset+1]
	chunkText := strings.Join(chunkWords, " ")

	return TextChunkMessage{Metadata: metadata, Content: chunkText}, nil
}

func (s *crawlingServer) GetChunksFromMicrosoft(ctx context.Context, req *pb.GetChunksFromMicrosoftRequest) (*pb.GetChunksFromMicrosoftResponse, error) {
	accessToken, err := s.retrieveAccessToken(ctx, req.UserId, "MICROSOFT")
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve access token: %w", err)
	}

	client := createMicrosoftOAuthClient(ctx, accessToken)
	type chunkResult struct {
		chunk *pb.TextChunkMessage
		err   error
	}
	numWorkers, err := strconv.Atoi(os.Getenv("CRAWLING_MICROSOFT_RETRIVAL_MAX_WORKERS"))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve the k value from the env variables: %w", err)
	}
	resultChan := make(chan chunkResult, len(req.Metadatas))
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(start int) {
			defer wg.Done()
			for j := start; j < len(req.Metadatas); j += numWorkers {
				metadata := req.Metadatas[j]
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
					Platform:         "MICROSOFT",
					Service:          metadata.Service,
				}

				chunk, err := RetrieveMicrosoftCrawler(ctx, client, internalMetadata)
				if err != nil {
					resultChan <- chunkResult{
						err: fmt.Errorf("error retrieving chunk for %s: %w", internalMetadata.FilePath, err),
					}
					continue
				}

				protoChunk := &pb.TextChunkMessage{
					Metadata: s.convertToProtoMetadata(chunk.Metadata),
					Content:  chunk.Content,
				}
				resultChan <- chunkResult{chunk: protoChunk}
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

	return &pb.GetChunksFromMicrosoftResponse{
		NumChunks: int64(len(chunks)),
		Chunks:    chunks,
	}, nil
}
