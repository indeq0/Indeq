package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type CrawlResult struct {
	Files []File
	Err   error
}

func (s *crawlingServer) CrawlGmail(ctx context.Context, client *http.Client, userID string) error {
	retrievalToken, err := s.ProcessGmailMessages(ctx, client, userID)
	if err != nil {
		return fmt.Errorf("error processing Google Gmail messages: %w", err)
	}

	if err := StoreGoogleGmailToken(ctx, s.db, userID, retrievalToken); err != nil {
		return fmt.Errorf("failed to store change token: %w", err)
	}
	return nil
}

func (s *crawlingServer) UpdateCrawlGmail(ctx context.Context, client *http.Client, userID string, retrievalToken string) (string, error) {
	tokenUint64, err := strconv.ParseUint(retrievalToken, 10, 64)
	if err != nil {
		newToken, err := s.ProcessGmailMessages(ctx, client, userID)
		if err != nil {
			return "", err
		}
		return newToken, nil
	}
	newRetrievalToken, err := s.CrawlGmailHistory(ctx, client, userID, tokenUint64)
	if err != nil {
		if strings.Contains(err.Error(), "Error 404") ||
			strings.Contains(err.Error(), "failedPrecondition") {
			return retrievalToken, nil
		}
		return "", err
	}
	return newRetrievalToken, nil
}

func (s *crawlingServer) CrawlGmailHistory(ctx context.Context, client *http.Client, userID string, lastHistoryID uint64) (string, error) {
	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return "", fmt.Errorf("failed to create Gmail service: %w", err)
	}

	var latestHistoryID = lastHistoryID
	historyCall := srv.Users.History.List("me").
		StartHistoryId(lastHistoryID).
		Fields("history(messagesAdded,messagesDeleted,labelsAdded,labelsRemoved),nextPageToken")

	pageToken := ""
	for {
		if pageToken != "" {
			historyCall.PageToken(pageToken)
		}

		if err := rateLimiter.Wait(ctx, "GOOGLE_GMAIL", userID); err != nil {
			return "", err
		}

		res, err := historyCall.Do()
		if err != nil {
			return "", fmt.Errorf("failed to fetch Gmail history: %w", err)
		}

		for _, history := range res.History {
			if len(history.MessagesAdded) > 0 {
				for _, msg := range history.MessagesAdded {
					fullMsg, err := srv.Users.Messages.Get("me", msg.Message.Id).
						Fields("id,internalDate,payload(headers,body,parts),historyId").
						Do()
					if err != nil {
						continue
					}
					file, err := processMessage(fullMsg, userID)
					if err == nil {
						if len(file.File) > 0 && s.isFileProcessed(userID, file.File[0].Metadata.ResourceID, "GOOGLE") {
							continue
						}

						for _, chunk := range file.File {
							if err := s.sendChunkToVector(ctx, chunk); err != nil {
								continue
							}
						}
						if len(file.File) > 0 {
							if err := s.sendFileDoneSignal(ctx, userID, file.File[0].Metadata.FilePath, "GOOGLE"); err != nil {
								continue
							}
						}

						if fullMsg.HistoryId > latestHistoryID {
							latestHistoryID = fullMsg.HistoryId
						}
					}
				}
			}
		}

		if res.NextPageToken == "" {
			break
		}
		pageToken = res.NextPageToken
	}

	retrievalToken := strconv.FormatUint(latestHistoryID, 10)
	return retrievalToken, nil
}

func (s *crawlingServer) ProcessGmailMessages(ctx context.Context, client *http.Client, userID string) (string, error) {
	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return "", fmt.Errorf("failed to create Gmail service: %w", err)
	}
	workers, err := strconv.Atoi(os.Getenv("CRAWLING_GMAIL_MAX_WORKERS"))
	if err != nil {
		return "", fmt.Errorf("failed to retrieve the gmail max workers value from the env variables: %w", err)
	}

	const pageSize = 1000

	var files []File
	var mu sync.Mutex
	var latestHistoryID uint64

	pageToken := ""
	workerChan := make(chan *gmail.Message, workers*10)
	resultChan := make(chan CrawlResult, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for msg := range workerChan {
				file, err := processMessage(msg, userID)
				if err != nil {
					resultChan <- CrawlResult{Err: fmt.Errorf("failed to process message: %v", err)}
					continue
				}

				if len(file.File) > 0 && s.isFileProcessed(userID, file.File[0].Metadata.ResourceID, "GOOGLE") {
					resultChan <- CrawlResult{Files: []File{file}}
					continue
				}

				for _, chunk := range file.File {
					if err := s.sendChunkToVector(ctx, chunk); err != nil {
						continue
					}
				}

				if len(file.File) > 0 {
					if err := s.sendFileDoneSignal(ctx, userID, file.File[0].Metadata.FilePath, "GOOGLE"); err != nil {
						continue
					}
				}

				mu.Lock()
				files = append(files, file)
				if msg.HistoryId > latestHistoryID {
					latestHistoryID = msg.HistoryId
				}
				mu.Unlock()

				resultChan <- CrawlResult{Files: []File{file}}
			}
		}()
	}

	for {
		if err := rateLimiter.Wait(ctx, "GOOGLE_GMAIL", userID); err != nil {
			break
		}

		call := srv.Users.Messages.List("me").
			Q("category:primary").
			PageToken(pageToken).
			MaxResults(pageSize).
			Fields("messages(id),nextPageToken")

		res, err := call.Do()
		if err != nil {
			break
		}

		for _, msg := range res.Messages {
			fullMsg, err := srv.Users.Messages.Get("me", msg.Id).
				Fields("id,internalDate,historyId,payload(headers,body/data,parts)").
				Do()
			if err != nil {
				continue
			}
			workerChan <- fullMsg
		}

		if res.NextPageToken == "" {
			break
		}
		pageToken = res.NextPageToken
	}

	close(workerChan)

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var errs []error
	for result := range resultChan {
		if result.Err != nil {
			errs = append(errs, result.Err)
		}
	}

	if len(errs) > 0 {
		return "", fmt.Errorf("some messages failed to process: %v", errs)
	}

	retrievalToken := strconv.FormatUint(latestHistoryID, 10)
	return retrievalToken, nil
}

// processMessage processes a single email message
func processMessage(fullMsg *gmail.Message, userID string) (File, error) {
	var subject, from string
	for _, header := range fullMsg.Payload.Headers {
		switch header.Name {
		case "Subject":
			subject = header.Value
		case "From":
			from = header.Value
		}
	}

	var bodyContent string
	if fullMsg.Payload.Body != nil && fullMsg.Payload.Body.Data != "" {
		bodyContent = decodeBase64Url(fullMsg.Payload.Body.Data)
	} else {
		bodyContent = extractBodyFromParts(fullMsg.Payload.Parts)
	}

	createdTime := time.UnixMilli(fullMsg.InternalDate)

	const chunkSize = 1000
	var chunks []TextChunkMessage
	content := []rune(bodyContent)

	for i := 0; i < len(content); i += chunkSize {
		end := i + chunkSize
		if end > len(content) {
			end = len(content)
		}

		chunkID := fmt.Sprintf("start:%d-end:%d", i, end)
		chunkContent := string(content[i:end])

		metadata := Metadata{
			DateCreated:      createdTime,
			DateLastModified: createdTime,
			UserID:           userID,
			ResourceID:       fullMsg.Id,
			Title:            subject,
			ResourceType:     "gmail/message",
			ChunkID:          chunkID,
			FileURL:          "https://mail.google.com/mail/u/0/#inbox/" + fullMsg.Id,
			FilePath:         from,
			Platform:         "GOOGLE",
			Service:          "GOOGLE_GMAIL",
		}

		chunks = append(chunks, TextChunkMessage{
			Metadata: metadata,
			Content:  chunkContent,
		})
	}

	if len(chunks) == 0 {
		metadata := Metadata{
			DateCreated:      createdTime,
			DateLastModified: createdTime,
			UserID:           userID,
			ResourceID:       fullMsg.Id,
			Title:            subject,
			ResourceType:     "gmail/message",
			ChunkID:          fmt.Sprintf("%s_chunk_0", fullMsg.Id),
			FileURL:          "https://mail.google.com/mail/u/0/#inbox/" + fullMsg.Id,
			FilePath:         from,
			Platform:         "GOOGLE",
			Service:          "GOOGLE_GMAIL",
		}
		chunks = append(chunks, TextChunkMessage{
			Metadata: metadata,
			Content:  "",
		})
	}
	return File{File: chunks}, nil
}

// Decode base64url encoded data
func decodeBase64Url(encoded string) string {
	decoded, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		fmt.Println("Failed to decode base64url:", err)
		return ""
	}
	return string(decoded)
}

// Extract body content from the message parts
func extractBodyFromParts(parts []*gmail.MessagePart) string {
	if parts == nil {
		return ""
	}

	var content string
	for _, part := range parts {
		if part.MimeType == "text/plain" && part.Body != nil && part.Body.Data != "" {
			content += decodeBase64Url(part.Body.Data) + "\n"
		} else if part.Parts != nil {
			content += extractBodyFromParts(part.Parts)
		}
	}
	return content
}

// RetrieveFromGmail retrieves a specific email chunk by resource ID and chunk boundaries
func RetrieveFromGmail(ctx context.Context, client *http.Client, metadata Metadata) (TextChunkMessage, error) {
	var startPos, endPos int
	if _, err := fmt.Sscanf(metadata.ChunkID, "start:%d-end:%d", &startPos, &endPos); err != nil {
		chunkNumStr := strings.TrimPrefix(metadata.ChunkID, metadata.ResourceID+"_chunk_")
		chunkNum, err := strconv.ParseUint(chunkNumStr, 10, 64)
		if err != nil {
			return TextChunkMessage{}, fmt.Errorf("invalid chunk ID format: %w", err)
		}
		const chunkSize = 1000
		startPos = int(chunkNum * chunkSize)
		endPos = startPos + chunkSize
	}

	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return TextChunkMessage{}, fmt.Errorf("failed to create Gmail service: %w", err)
	}

	fullMsg, err := srv.Users.Messages.Get("me", metadata.ResourceID).
		Fields("id,internalDate,payload(headers,body,parts)").
		Do()
	if err != nil {
		return TextChunkMessage{}, fmt.Errorf("failed to retrieve email with ID %s: %w", metadata.ResourceID, err)
	}

	var subject, from string
	for _, header := range fullMsg.Payload.Headers {
		switch header.Name {
		case "Subject":
			subject = header.Value
		case "From":
			from = header.Value
		}
	}

	var bodyContent string
	if fullMsg.Payload.Body != nil && fullMsg.Payload.Body.Data != "" {
		bodyContent = decodeBase64Url(fullMsg.Payload.Body.Data)
	} else {
		bodyContent = extractBodyFromParts(fullMsg.Payload.Parts)
	}

	content := []rune(bodyContent)
	if startPos >= len(content) {
		return TextChunkMessage{}, fmt.Errorf("start position %d exceeds content length %d", startPos, len(content))
	}

	if endPos > len(content) {
		endPos = len(content)
	}

	chunkContent := string(content[startPos:endPos])

	createdTime := time.UnixMilli(fullMsg.InternalDate)

	resultMetadata := Metadata{
		DateCreated:      createdTime,
		DateLastModified: createdTime,
		UserID:           metadata.UserID,
		ResourceID:       fullMsg.Id,
		Title:            subject,
		ResourceType:     "gmail/message",
		ChunkID:          metadata.ChunkID,
		FileURL:          "https://mail.google.com/mail/u/0/#inbox/" + fullMsg.Id,
		FilePath:         from,
		Platform:         "GOOGLE",
		Service:          "GOOGLE_GMAIL",
	}
	return TextChunkMessage{
		Metadata: resultMetadata,
		Content:  chunkContent,
	}, nil
}
