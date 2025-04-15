package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	pb "github.com/cc-0000/indeq/common/api"
	"golang.org/x/oauth2"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const NotionVersion = "2022-06-28"

// Notion API types
type Block struct {
	ID             string                 `json:"id"`
	Type           string                 `json:"type"`
	CreatedTime    time.Time              `json:"created_time"`
	LastEditedTime time.Time              `json:"last_edited_time"`
	HasChildren    bool                   `json:"has_children"`
	Content        map[string]interface{} `json:"-"`
}

func (b *Block) UnmarshalJSON(data []byte) error {
	type Alias Block
	aux := &struct {
		*Alias
	}{Alias: (*Alias)(b)}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if body, ok := raw[b.Type]; ok {
		var content map[string]interface{}
		if err := json.Unmarshal(body, &content); err != nil {
			return err
		}
		b.Content = content
	}

	return nil
}

type RichText struct {
	Type string `json:"type"`
	Text struct {
		Content string `json:"content"`
		Link    *struct {
			URL string `json:"url"`
		} `json:"link"`
	} `json:"text"`
	PlainText string  `json:"plain_text"`
	Href      *string `json:"href"`
}

type NotionObject struct {
	ID             string    `json:"id"`
	Object         string    `json:"object"`
	CreatedTime    time.Time `json:"created_time"`
	LastEditedTime time.Time `json:"last_edited_time"`
	URL            string    `json:"url"`
	Parent         struct {
		Type string `json:"type"`
	} `json:"parent"`
	Properties map[string]interface{} `json:"properties"`
}

type NotionDatabase struct {
	ID             string                 `json:"id"`
	Object         string                 `json:"object"`
	CreatedTime    time.Time              `json:"created_time"`
	LastEditedTime time.Time              `json:"last_edited_time"`
	URL            string                 `json:"url"`
	Title          []RichText             `json:"title"`
	Description    []RichText             `json:"description"`
	Properties     map[string]interface{} `json:"properties"`
	Parent         struct {
		Type   string `json:"type"`
		PageID string `json:"page_id"`
	} `json:"parent"`
}

// API response types
type NotionSearchResponse struct {
	Results    []NotionObject `json:"results"`
	NextCursor string         `json:"next_cursor"`
	HasMore    bool           `json:"has_more"`
}

type NotionPageResponse struct {
	Results []Block `json:"results"`
}

type NotionDatabaseResponse struct {
	Results    []map[string]interface{} `json:"results"`
	NextCursor string                   `json:"next_cursor"`
	HasMore    bool                     `json:"has_more"`
}

// ProcessedBlock represents a processed block with its content and ID
type ProcessedBlock struct {
	ID          string
	Content     string
	Words       []string
	StartOffset int
	WordCount   int
}

// NotionChunker manages the chunking process
type NotionChunker struct {
	ChunkSize int
	Overlap   int
	Blocks    []ProcessedBlock
}

type chunkResult struct {
	chunk *pb.TextChunkMessage
	err   error
}

// Utility Functions

// parseChunkID parses a chunk ID string into structured values.
func parseChunkID(chunkID string) (startBlockID string, startOffset int, endBlockID string, endOffset int, err error) {
	parts := strings.Split(chunkID, ";")
	chunkIDMap := make(map[string]string)
	for _, part := range parts {
		kv := strings.Split(part, "=")
		if len(kv) != 2 {
			return "", 0, "", 0, fmt.Errorf("invalid chunk ID format: malformed pair in '%s'", part)
		}
		chunkIDMap[kv[0]] = kv[1]
	}
	startBlockID = chunkIDMap["start_block"]
	endBlockID = chunkIDMap["end_block"]

	startOffset, err = strconv.Atoi(chunkIDMap["start_offset"])
	if err != nil {
		return "", 0, "", 0, fmt.Errorf("invalid start offset: %w", err)
	}
	endOffset, err = strconv.Atoi(chunkIDMap["end_offset"])
	if err != nil {
		return "", 0, "", 0, fmt.Errorf("invalid end offset: %w", err)
	}

	if startBlockID == "" || endBlockID == "" {
		return "", 0, "", 0, fmt.Errorf("missing block IDs in chunk ID")
	}

	return startBlockID, startOffset, endBlockID, endOffset, nil
}

// injectMetadataBlock creates a Notion paragraph block from metadata
func injectMetadataBlock(id, content string, createdTime, lastEditedTime time.Time) Block {
	return Block{
		ID:             id,
		Type:           "paragraph",
		CreatedTime:    createdTime,
		LastEditedTime: lastEditedTime,
		Content: map[string]interface{}{
			"rich_text": []map[string]interface{}{
				{
					"type": "text",
					"text": map[string]interface{}{
						"content": content,
					},
					"plain_text": content,
				},
			},
		},
	}
}

// NewNotionChunker creates a new chunker
func NewNotionChunker(chunkSize, overlap int) *NotionChunker {
	return &NotionChunker{
		ChunkSize: chunkSize,
		Overlap:   overlap,
		Blocks:    []ProcessedBlock{},
	}
}

// createNotionOAuthClient creates a new OAuth client for Notion API
func createNotionOAuthClient(ctx context.Context, accessToken string) *http.Client {
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
	return oauth2.NewClient(ctx, tokenSource)
}

// NotionCrawler is the main entry point for crawling Notion content
func (s *crawlingServer) NotionCrawler(ctx context.Context, client *http.Client, userID string) error {
	files, retrievalToken, err := s.NotionSearch(ctx, client, userID, "", true)
	if err != nil {
		return fmt.Errorf("failed to search for objects: %w", err)
	}

	if err := StoreNotionToken(ctx, s.db, userID, retrievalToken); err != nil {
		return fmt.Errorf("failed to store retrieval token: %w", err)
	}

	if err := s.processNotionFiles(ctx, client, files); err != nil {
		return fmt.Errorf("failed to process files: %w", err)
	}

	if err := s.sendCrawlDoneSignal(ctx, userID, "NOTION"); err != nil {
		return fmt.Errorf("failed to send crawl done signal: %w", err)
	}

	return nil
}

func (s *crawlingServer) UpdateCrawlNotion(ctx context.Context, client *http.Client, userID string, retrievalToken string) (string, error) {
	files, newToken, err := s.NotionSearch(ctx, client, userID, retrievalToken, false)
	if err != nil {
		return "", fmt.Errorf("failed to search for objects: %w", err)
	}

	var filePaths []string
	for _, file := range files.Files {
		if len(file.File) == 0 {
			continue
		}

		resourceID := file.File[0].Metadata.ResourceID
		filePath := file.File[0].Metadata.FilePath

		if err := s.DeleteChunkMappingsForFile(ctx, userID, "NOTION", resourceID); err != nil {
			log.Printf("Warning: failed to delete old chunk mappings for file %s: %v", resourceID, err)
			continue
		}

		if err := UpsertProcessingStatus(ctx, s.db, userID, resourceID, "NOTION", false); err != nil {
			log.Printf("Warning: failed to reset processing status for file %s: %v", resourceID, err)
			continue
		}

		filePaths = append(filePaths, filePath)
	}

	if len(filePaths) > 0 {
		if _, err = s.vectorService.DeleteFiles(ctx, &pb.VectorFileDeleteRequest{
			UserId:    userID,
			Platform:  pb.Platform_PLATFORM_NOTION,
			Files:     filePaths,
			Exclusive: false,
		}); err != nil {
			return "", fmt.Errorf("failed to delete old vectors: %w", err)
		}
	}

	if err := s.processNotionFiles(ctx, client, files); err != nil {

		for _, file := range files.Files {
			if len(file.File) > 0 {
				_ = UpsertProcessingStatus(ctx, s.db, userID, file.File[0].Metadata.ResourceID, "NOTION", false)
			}
		}
		return "", fmt.Errorf("failed to process files: %w", err)
	}

	if len(files.Files) > 0 {
		if err := s.sendCrawlDoneSignal(ctx, userID, "NOTION"); err != nil {
			return "", fmt.Errorf("failed to send crawl done signal: %w", err)
		}
	}

	return newToken, nil
}

// ProcessBlocks processes the blocks from a Notion API response
func (nc *NotionChunker) ProcessBlocks(blockResponse NotionPageResponse) []ProcessedBlock {
	nc.Blocks = []ProcessedBlock{}
	currentOffset := 0

	for _, block := range blockResponse.Results {
		blockContent := ""

		switch block.Type {
		case "paragraph", "heading_1", "heading_2", "heading_3", "bulleted_list_item", "numbered_list_item", "to_do", "toggle":
			if richTextArr, ok := block.Content["rich_text"].([]interface{}); ok {
				for _, rt := range richTextArr {
					if rtMap, ok := rt.(map[string]interface{}); ok {
						if plainText, ok := rtMap["plain_text"].(string); ok {
							if block.Type == "bulleted_list_item" {
								blockContent += "• " + plainText + "\n"
							} else if block.Type == "numbered_list_item" || block.Type == "to_do" {
								prefix := ""
								if block.Type == "to_do" {
									prefix = "☐ "
								}
								blockContent += prefix + plainText + "\n"
							} else {
								blockContent += plainText + " "
							}
						}
					}
				}
			}
		case "image", "video", "file", "pdf":
			if url, ok := block.Content["url"].(string); ok {
				blockContent += fmt.Sprintf("%s: %s\n", cases.Title(language.English).String(block.Type), url)
			}
		case "divider":
			blockContent += "Divider\n"
		case "child_database":
			blockContent += "Child Database\n"
		case "child_page":
			blockContent += "Child Page\n"
		}

		words := strings.Fields(blockContent)
		if len(words) > 0 {
			nc.Blocks = append(nc.Blocks, ProcessedBlock{
				ID:          block.ID,
				Content:     blockContent,
				Words:       words,
				StartOffset: currentOffset,
				WordCount:   len(words),
			})
			currentOffset += len(words)
		}
	}

	return nc.Blocks
}

// GenerateChunks creates chunks with the specified size and overlap
func (nc *NotionChunker) GenerateChunks(pageResponse NotionObject, userID, pageTitle string) []TextChunkMessage {
	if pageTitle == "" {
		pageTitle = "Untitled"
	}
	var chunks []TextChunkMessage

	totalWords := 0
	for _, block := range nc.Blocks {
		totalWords += len(block.Words)
	}

	stepSize := nc.ChunkSize - nc.Overlap

	if totalWords == 0 {
		return chunks
	}

	for chunkStart := 0; chunkStart < totalWords; chunkStart += stepSize {
		chunkEnd := chunkStart + nc.ChunkSize
		if chunkEnd > totalWords {
			chunkEnd = totalWords
		}

		var chunkWords []string
		var startBlock, endBlock *ProcessedBlock
		var startWordOffset, endWordOffset int

		for _, block := range nc.Blocks {
			if block.StartOffset <= chunkStart &&
				block.StartOffset+block.WordCount > chunkStart {
				startBlock = &block
				startWordOffset = chunkStart - block.StartOffset
				break
			}
		}

		for _, block := range nc.Blocks {
			if block.StartOffset < chunkEnd &&
				block.StartOffset+block.WordCount >= chunkEnd {
				endBlock = &block
				endWordOffset = chunkEnd - block.StartOffset
				break
			}
		}

		if startBlock == nil || endBlock == nil {
			continue
		}

		for _, block := range nc.Blocks {
			if block.StartOffset >= startBlock.StartOffset &&
				block.StartOffset+block.WordCount <= endBlock.StartOffset+endBlock.WordCount {
				if block.ID == startBlock.ID {
					if len(block.Words) > startWordOffset {
						chunkWords = append(chunkWords, block.Words[startWordOffset:]...)
					}
				} else if block.ID == endBlock.ID {
					if endWordOffset > 0 && endWordOffset <= len(block.Words) {
						chunkWords = append(chunkWords, block.Words[:endWordOffset]...)
					}
				} else {
					chunkWords = append(chunkWords, block.Words...)
				}
			}
		}

		if len(chunkWords) > 0 {
			chunkID := fmt.Sprintf("start_block=%s;start_offset=%d;end_block=%s;end_offset=%d",
				startBlock.ID, startWordOffset,
				endBlock.ID, endWordOffset)
			chunk := TextChunkMessage{
				Metadata: Metadata{
					UserID:           userID,
					ResourceID:       pageResponse.ID,
					ResourceType:     pageResponse.Object,
					Platform:         "NOTION",
					Service:          "NOTION",
					DateCreated:      pageResponse.CreatedTime,
					DateLastModified: pageResponse.LastEditedTime,
					Title:            pageTitle,
					ChunkID:          chunkID,
					FilePath:         "/" + pageTitle,
					FileURL:          pageResponse.URL,
				},
				Content: strings.Join(chunkWords, " "),
			}
			chunks = append(chunks, chunk)
		}
	}

	return chunks
}

// NotionSearch searches for all accessible Notion pages and databases
func (s *crawlingServer) NotionSearch(ctx context.Context, client *http.Client, userID, retrievalToken string, initial bool) (ListofFiles, string, error) {
	var files ListofFiles
	nextCursor := ""
	newRetrievalToken := ""

	for {
		searchBody := map[string]interface{}{
			"sort": map[string]interface{}{
				"direction": "ascending",
				"timestamp": "last_edited_time",
			},
			"page_size": 100,
		}
		if nextCursor != "" {
			searchBody["start_cursor"] = nextCursor
		}

		if err := rateLimiter.Wait(ctx, "NOTION", userID); err != nil {
			return files, retrievalToken, fmt.Errorf("rate limit wait failed: %w", err)
		}

		bodyBytes, err := json.Marshal(searchBody)
		if err != nil {
			return files, retrievalToken, fmt.Errorf("failed to marshal search body: %w", err)
		}

		req, err := http.NewRequest("POST", "https://api.notion.com/v1/search", bytes.NewBuffer(bodyBytes))
		if err != nil {
			return files, retrievalToken, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Notion-Version", NotionVersion)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return files, retrievalToken, fmt.Errorf("failed to send request: %w", err)
		}
		defer resp.Body.Close()

		var searchResp NotionSearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
			return files, retrievalToken, fmt.Errorf("failed to decode search response: %w", err)
		}
		for _, page := range searchResp.Results {
			if retrievalToken != "" && !initial {
				retrivalTime, err := time.Parse(time.RFC3339, retrievalToken)
				if err != nil {
					return files, retrievalToken, fmt.Errorf("failed to parse retrieval token: %w", err)
				}
				if page.LastEditedTime.Before(retrivalTime) {
					continue
				}
				if page.LastEditedTime.Equal(retrivalTime) {
					continue
				}
			}
			metadata := Metadata{
				DateCreated:      page.CreatedTime,
				DateLastModified: page.LastEditedTime,
				UserID:           userID,
				ResourceID:       page.ID,
				ResourceType:     page.Object,
				FileURL:          page.URL,
				FilePath:         "",
				Title:            "",
				Platform:         "NOTION",
				Service:          "NOTION",
			}

			file := File{
				File: []TextChunkMessage{
					{
						Metadata: metadata,
						Content:  "",
					},
				},
			}
			files.Files = append(files.Files, file)
			newRetrievalToken = page.LastEditedTime.Format(time.RFC3339)
		}

		if !searchResp.HasMore {
			break
		}
		nextCursor = searchResp.NextCursor
	}

	if len(files.Files) == 0 {
		newRetrievalToken = retrievalToken
	}
	return files, newRetrievalToken, nil
}

func (s *crawlingServer) processNotionFiles(ctx context.Context, client *http.Client, files ListofFiles) error {
	if len(files.Files) == 0 {
		return nil
	}

	var processedFiles ListofFiles
	type processResult struct {
		file File
		err  error
	}

	resultChan := make(chan processResult, len(files.Files))
	var wg sync.WaitGroup

	for _, file := range files.Files {
		if len(file.File) == 0 {
			continue
		}

		if file.File[0].Metadata.DateLastModified.IsZero() {
			if s.isFileProcessed(file.File[0].Metadata.UserID, file.File[0].Metadata.ResourceID, "NOTION") {
				continue
			}
		}

		f := file
		wg.Add(1)
		go func() {
			defer wg.Done()
			var result processResult

			switch f.File[0].Metadata.ResourceType {
			case "page":
				pageFile := s.processNotionPage(ctx, client, f)
				if len(pageFile.File) > 0 {
					for _, chunk := range pageFile.File {
						shortKey, err := s.AddChunkMapping(ctx, chunk.Metadata.UserID, "NOTION", chunk.Metadata.ChunkID, f.File[0].Metadata.ResourceID, "NOTION")
						if err != nil {
							continue
						}
						chunk.Metadata.ChunkID = shortKey
						if err := s.sendChunkToVector(ctx, chunk); err != nil {
							continue
						}
					}
					s.markFileProcessed(f.File[0].Metadata.UserID, f.File[0].Metadata.ResourceID, "NOTION")
					_ = s.sendFileDoneSignal(ctx, f.File[0].Metadata.UserID, f.File[0].Metadata.FilePath, "NOTION")
					result.file = pageFile
				}
			case "database":
				dbFile := s.processNotionDatabase(ctx, client, f)
				if len(dbFile.File) > 0 {
					for _, chunk := range dbFile.File {
						shortKey, err := s.AddChunkMapping(ctx, chunk.Metadata.UserID, "NOTION", chunk.Metadata.ChunkID, f.File[0].Metadata.ResourceID, "NOTION")
						if err != nil {
							continue
						}
						chunk.Metadata.ChunkID = shortKey
						if err := s.sendChunkToVector(ctx, chunk); err != nil {
							continue
						}
					}
					s.markFileProcessed(f.File[0].Metadata.UserID, f.File[0].Metadata.ResourceID, "NOTION")
					_ = s.sendFileDoneSignal(ctx, f.File[0].Metadata.UserID, f.File[0].Metadata.FilePath, "NOTION")
					result.file = dbFile
				}
			default:
				result.err = fmt.Errorf("unsupported resource type: %s", f.File[0].Metadata.ResourceType)
			}
			resultChan <- result
		}()
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var errs []error
	for result := range resultChan {
		if result.err != nil {
			errs = append(errs, result.err)
			continue
		}
		if len(result.file.File) > 0 {
			processedFiles.Files = append(processedFiles.Files, result.file)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("some files failed to process: %v", errs)
	}

	if len(processedFiles.Files) > 0 {
		_ = s.sendCrawlDoneSignal(ctx, processedFiles.Files[0].File[0].Metadata.UserID, "NOTION")
	}
	return nil
}

// fetchNotionResource makes a GET request to the Notion API and decodes the response
func fetchNotionResource(ctx context.Context, client *http.Client, url string, result interface{}) error {
	if err := rateLimiter.Wait(ctx, "NOTION", ""); err != nil {
		return fmt.Errorf("rate limit wait failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Notion-Version", NotionVersion)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("request failed with status %d and error reading body: %w", resp.StatusCode, err)
		}
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	return nil
}

func (s *crawlingServer) processNotionPage(ctx context.Context, client *http.Client, file File) File {
	if len(file.File) == 0 {
		return File{}
	}
	pageID := file.File[0].Metadata.ResourceID

	var pageResponse NotionObject
	if err := fetchNotionResource(ctx, client, fmt.Sprintf("https://api.notion.com/v1/pages/%s", pageID), &pageResponse); err != nil {
		log.Printf("Error fetching page: %v", err)
		return File{}
	}

	var blockResponse NotionPageResponse
	if err := fetchNotionResource(ctx, client, fmt.Sprintf("https://api.notion.com/v1/blocks/%s/children", pageID), &blockResponse); err != nil {
		log.Printf("Error fetching blocks: %v", err)
		return File{}
	}

	var pageTitle string
	var propertiesContent string

	for propName, propValue := range pageResponse.Properties {
		propObj, ok := propValue.(map[string]interface{})
		if !ok {
			continue
		}

		propType, ok := propObj["type"].(string)
		if !ok {
			continue
		}

		value := extractPropertyValue(propType, propObj)
		if value != "" {
			propertiesContent += fmt.Sprintf("%s: %s\n", propName, value)
			if propType == "title" {
				pageTitle = value
			}
		}
	}

	if propertiesContent != "" {
		blockResponse.Results = append([]Block{
			{
				ID:             fmt.Sprintf("%s_properties", pageID),
				Type:           "paragraph",
				CreatedTime:    pageResponse.CreatedTime,
				LastEditedTime: pageResponse.LastEditedTime,
				Content: map[string]interface{}{
					"rich_text": []map[string]interface{}{
						{
							"type": "text",
							"text": map[string]interface{}{
								"content": "Page Properties:\n" + propertiesContent,
							},
							"plain_text": "Page Properties:\n" + propertiesContent,
						},
					},
				},
			},
		}, blockResponse.Results...)
	}

	chunker := NewNotionChunker(400, 80)
	chunker.ProcessBlocks(blockResponse)
	chunks := chunker.GenerateChunks(pageResponse, file.File[0].Metadata.UserID, pageTitle)

	file.File = chunks
	return file
}

func (s *crawlingServer) processNotionDatabase(ctx context.Context, client *http.Client, file File) File {
	dbID := file.File[0].Metadata.ResourceID

	var dbResponse NotionDatabase
	if err := fetchNotionResource(ctx, client, fmt.Sprintf("https://api.notion.com/v1/databases/%s", dbID), &dbResponse); err != nil {
		log.Printf("Error fetching database: %v", err)
		return File{}
	}

	dbTitle := ""
	for _, text := range dbResponse.Title {
		dbTitle += text.PlainText
	}

	var blockResponse NotionPageResponse

	if dbTitle != "" {
		blockResponse.Results = append(blockResponse.Results, Block{
			ID:             fmt.Sprintf("%s_title", dbID),
			Type:           "heading_1",
			CreatedTime:    dbResponse.CreatedTime,
			LastEditedTime: dbResponse.LastEditedTime,
			Content: map[string]interface{}{
				"rich_text": []map[string]interface{}{
					{
						"type": "text",
						"text": map[string]interface{}{
							"content": dbTitle,
						},
						"plain_text": dbTitle,
					},
				},
			},
		})
	}

	if len(dbResponse.Description) > 0 {
		var descriptionText string
		for _, text := range dbResponse.Description {
			descriptionText += text.PlainText + " "
		}
		if descriptionText != "" {
			blockResponse.Results = append(blockResponse.Results, Block{
				ID:             fmt.Sprintf("%s_description", dbID),
				Type:           "paragraph",
				CreatedTime:    dbResponse.CreatedTime,
				LastEditedTime: dbResponse.LastEditedTime,
				Content: map[string]interface{}{
					"rich_text": []map[string]interface{}{
						{
							"type": "text",
							"text": map[string]interface{}{
								"content": descriptionText,
							},
							"plain_text": descriptionText,
						},
					},
				},
			})
		}
	}

	nextCursor := ""
	rowCounter := 0
	for {
		queryBody := map[string]interface{}{
			"page_size": 100,
		}
		if nextCursor != "" {
			queryBody["start_cursor"] = nextCursor
		}

		if err := rateLimiter.Wait(ctx, "NOTION", file.File[0].Metadata.UserID); err != nil {
			fmt.Println("Rate limit wait failed:", err)
			break
		}

		bodyBytes, err := json.Marshal(queryBody)
		if err != nil {
			fmt.Println("Error marshaling query body:", err)
			break
		}

		queryReq, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("https://api.notion.com/v1/databases/%s/query", dbID), bytes.NewBuffer(bodyBytes))
		if err != nil {
			fmt.Println("Error creating query request:", err)
			break
		}
		queryReq.Header.Set("Notion-Version", NotionVersion)
		queryReq.Header.Set("Content-Type", "application/json")

		queryResp, err := client.Do(queryReq)
		if err != nil {
			fmt.Println("Error executing query request:", err)
			break
		}

		var dbQueryResponse NotionDatabaseResponse
		if err := json.NewDecoder(queryResp.Body).Decode(&dbQueryResponse); err != nil {
			fmt.Println("Error decoding query response:", err)
			queryResp.Body.Close()
			break
		}
		queryResp.Body.Close()

		for _, row := range dbQueryResponse.Results {
			rowCounter++
			rowContent := ""
			properties, ok := row["properties"].(map[string]interface{})
			if !ok {
				continue
			}

			for propName, propValue := range properties {
				propObj, ok := propValue.(map[string]interface{})
				if !ok {
					continue
				}

				propType, ok := propObj["type"].(string)
				if !ok {
					continue
				}

				value := extractPropertyValue(propType, propObj)
				if value != "" {
					rowContent += fmt.Sprintf("%s: %s\n", propName, value)
				}
			}

			if rowContent != "" {
				blockResponse.Results = append(blockResponse.Results,
					injectMetadataBlock(
						fmt.Sprintf("%s_row_%d", dbID, rowCounter),
						rowContent,
						dbResponse.CreatedTime,
						dbResponse.LastEditedTime,
					),
				)
			}
		}

		if !dbQueryResponse.HasMore {
			break
		}
		nextCursor = dbQueryResponse.NextCursor
	}

	chunker := NewNotionChunker(400, 80)
	chunker.ProcessBlocks(blockResponse)
	chunks := chunker.GenerateChunks(NotionObject{
		ID:             dbResponse.ID,
		Object:         "database",
		CreatedTime:    dbResponse.CreatedTime,
		LastEditedTime: dbResponse.LastEditedTime,
		URL:            dbResponse.URL,
	}, file.File[0].Metadata.UserID, dbTitle)

	file.File = chunks
	return file
}

// extractPropertyValue extracts the value from a Notion property
func extractPropertyValue(propType string, propObj map[string]interface{}) string {
	switch propType {
	case "title":
		return extractRichTextValue(propObj["title"])
	case "rich_text":
		return extractRichTextValue(propObj["rich_text"])
	case "number":
		if number, ok := propObj["number"].(float64); ok {
			return fmt.Sprintf("%.2f", number)
		}
	case "select":
		if selectObj, ok := propObj["select"].(map[string]interface{}); ok {
			if name, ok := selectObj["name"].(string); ok {
				return name
			}
		}
	case "multi_select":
		if multiSelectArray, ok := propObj["multi_select"].([]interface{}); ok {
			var values []string
			for _, item := range multiSelectArray {
				if itemObj, ok := item.(map[string]interface{}); ok {
					if name, ok := itemObj["name"].(string); ok {
						values = append(values, name)
					}
				}
			}
			return strings.Join(values, ", ")
		}
	case "date":
		if dateObj, ok := propObj["date"].(map[string]interface{}); ok {
			start, _ := dateObj["start"].(string)
			end, _ := dateObj["end"].(string)
			if end != "" {
				return fmt.Sprintf("%s to %s", start, end)
			}
			return start
		}
	case "checkbox":
		if checkbox, ok := propObj["checkbox"].(bool); ok {
			return fmt.Sprintf("%v", checkbox)
		}
	}
	return ""
}

// extractRichTextValue extracts text from rich text array
func extractRichTextValue(richTextInterface interface{}) string {
	richTextArray, ok := richTextInterface.([]interface{})
	if !ok || len(richTextArray) == 0 {
		return ""
	}

	var result []string
	for _, item := range richTextArray {
		textObj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if plainText, ok := textObj["plain_text"].(string); ok {
			result = append(result, plainText)
		}
	}
	return strings.Join(result, " ")
}

// RetrieveNotionCrawler retrieves specific chunks based on metadata
func RetrieveNotionCrawler(ctx context.Context, client *http.Client, metadata Metadata) (TextChunkMessage, error) {
	if metadata.ResourceType == "page" {
		return retriveNotionPage(ctx, client, metadata)
	}
	if metadata.ResourceType == "database" {
		return retriveNotionDatabase(ctx, client, metadata)
	}
	return TextChunkMessage{}, fmt.Errorf("unsupported resource type: %s", metadata.ResourceType)
}

// GetChunksFromNotion handles the gRPC request to get chunks from Notion
func (s *crawlingServer) GetChunksFromNotion(ctx context.Context, req *pb.GetChunksFromNotionRequest) (*pb.GetChunksFromNotionResponse, error) {
	accessToken, err := s.retrieveAccessToken(ctx, req.UserId, "NOTION")
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve access token: %w", err)
	}

	client := createNotionOAuthClient(ctx, accessToken)
	resultChan := make(chan chunkResult, len(req.Metadatas))
	var wg sync.WaitGroup

	for _, meta := range req.Metadatas {
		wg.Add(1)
		go func(meta *pb.Metadata) {
			defer wg.Done()

			docID := fmt.Sprintf("%s_NOTION", meta.UserId)
			row := s.ChunkIDdb.Get(ctx, docID)
			if row.Err() != nil {
				resultChan <- chunkResult{nil, fmt.Errorf("failed to get chunk mappings: %w", row.Err())}
				return
			}

			var doc map[string]interface{}
			if err := row.ScanDoc(&doc); err != nil {
				resultChan <- chunkResult{nil, fmt.Errorf("failed to scan document: %w", err)}
				return
			}

			mappings, ok := doc["chunkMappings"].([]interface{})
			if !ok {
				resultChan <- chunkResult{nil, fmt.Errorf("invalid mappings format")}
				return
			}

			var originalChunkID string
			for _, mapping := range mappings {
				m, ok := mapping.(map[string]interface{})
				if !ok {
					continue
				}
				if m["shortKey"] == meta.ChunkId {
					originalChunkID = m["chunkId"].(string)
					break
				}
			}

			if originalChunkID == "" {
				resultChan <- chunkResult{nil, fmt.Errorf("short key not found: %s", meta.ChunkId)}
				return
			}

			internalMeta := Metadata{
				DateCreated:      time.Unix(meta.DateCreated.Seconds, int64(meta.DateCreated.Nanos)),
				DateLastModified: time.Unix(meta.DateLastModified.Seconds, int64(meta.DateLastModified.Nanos)),
				UserID:           meta.UserId,
				ResourceID:       meta.FileId,
				ResourceType:     meta.ResourceType,
				FileURL:          meta.FileUrl,
				FilePath:         meta.FilePath,
				Title:            meta.Title,
				ChunkID:          originalChunkID,
				Platform:         meta.Platform.String(),
				Service:          meta.Service,
			}

			chunk, err := RetrieveNotionCrawler(ctx, client, internalMeta)
			if err != nil {
				resultChan <- chunkResult{nil, fmt.Errorf("failed to retrieve chunk: %w", err)}
				return
			}

			chunk.Metadata.ChunkID = meta.ChunkId
			pbChunk := &pb.TextChunkMessage{
				Metadata: &pb.Metadata{
					DateCreated:      timestamppb.New(chunk.Metadata.DateCreated),
					DateLastModified: timestamppb.New(chunk.Metadata.DateLastModified),
					UserId:           chunk.Metadata.UserID,
					FileId:           chunk.Metadata.ResourceID,
					ResourceType:     chunk.Metadata.ResourceType,
					FileUrl:          chunk.Metadata.FileURL,
					FilePath:         chunk.Metadata.FilePath,
					Title:            chunk.Metadata.Title,
					ChunkId:          chunk.Metadata.ChunkID,
					Platform:         pb.Platform(pb.Platform_value[chunk.Metadata.Platform]),
					Service:          chunk.Metadata.Service,
				},
				Content: chunk.Content,
			}

			resultChan <- chunkResult{pbChunk, nil}
		}(meta)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var chunks []*pb.TextChunkMessage
	var errors []error

	for result := range resultChan {
		if result.err != nil {
			errors = append(errors, result.err)
			continue
		}
		if result.chunk != nil {
			chunks = append(chunks, result.chunk)
		}
	}

	if len(errors) > 0 {
		return nil, fmt.Errorf("failed to retrieve chunks: %v", errors)
	}

	return &pb.GetChunksFromNotionResponse{
		Chunks: chunks,
	}, nil
}

// retriveNotionPage retrieves a specific chunk from a Notion page
func retriveNotionPage(ctx context.Context, client *http.Client, metadata Metadata) (TextChunkMessage, error) {
	startBlockID, startOffset, endBlockID, endOffset, err := parseChunkID(metadata.ChunkID)
	if err != nil {
		return TextChunkMessage{}, fmt.Errorf("failed to parse chunk ID: %w", err)
	}

	pageID := metadata.ResourceID
	var pageResponse NotionObject
	if err := fetchNotionResource(ctx, client, fmt.Sprintf("https://api.notion.com/v1/pages/%s", pageID), &pageResponse); err != nil {
		return TextChunkMessage{}, fmt.Errorf("failed to fetch page: %w", err)
	}

	var blockResponse NotionPageResponse
	if err := fetchNotionResource(ctx, client, fmt.Sprintf("https://api.notion.com/v1/blocks/%s/children", pageID), &blockResponse); err != nil {
		return TextChunkMessage{}, fmt.Errorf("failed to fetch blocks: %w", err)
	}

	if startBlockID == fmt.Sprintf("%s_properties", pageID) ||
		(startBlockID == blockResponse.Results[0].ID && startOffset == 0) {
		var propertiesContent string
		for propName, propValue := range pageResponse.Properties {
			propObj, ok := propValue.(map[string]interface{})
			if !ok {
				continue
			}

			propType, ok := propObj["type"].(string)
			if !ok {
				continue
			}

			value := extractPropertyValue(propType, propObj)
			if value != "" {
				propertiesContent += fmt.Sprintf("%s: %s\n", propName, value)
			}
		}

		if propertiesContent != "" {
			blockResponse.Results = append([]Block{
				injectMetadataBlock(
					fmt.Sprintf("%s_properties", pageID),
					"Page Properties:\n"+propertiesContent,
					pageResponse.CreatedTime,
					pageResponse.LastEditedTime,
				),
			}, blockResponse.Results...)
		}
	}

	chunker := NewNotionChunker(400, 80)
	chunker.ProcessBlocks(blockResponse)

	var startBlock, endBlock *ProcessedBlock
	for i := range chunker.Blocks {
		if chunker.Blocks[i].ID == startBlockID {
			startBlock = &chunker.Blocks[i]
		}
		if chunker.Blocks[i].ID == endBlockID {
			endBlock = &chunker.Blocks[i]
		}
	}

	if startBlock == nil || endBlock == nil {
		return TextChunkMessage{}, fmt.Errorf("could not find start or end block")
	}

	var chunkWords []string
	for _, block := range chunker.Blocks {
		if block.StartOffset >= startBlock.StartOffset &&
			block.StartOffset+block.WordCount <= endBlock.StartOffset+endBlock.WordCount {
			if block.ID == startBlockID {
				if len(block.Words) > startOffset {
					chunkWords = append(chunkWords, block.Words[startOffset:]...)
				}
			} else if block.ID == endBlockID {
				if endOffset > 0 && endOffset <= len(block.Words) {
					chunkWords = append(chunkWords, block.Words[:endOffset]...)
				}
			} else {
				chunkWords = append(chunkWords, block.Words...)
			}
		}
	}

	chunk := TextChunkMessage{
		Metadata: metadata,
		Content:  strings.Join(chunkWords, " "),
	}
	return chunk, nil
}

// retriveNotionDatabase retrieves a specific chunk from a Notion database
func retriveNotionDatabase(ctx context.Context, client *http.Client, metadata Metadata) (TextChunkMessage, error) {
	startBlockID, startOffset, endBlockID, endOffset, err := parseChunkID(metadata.ChunkID)
	if err != nil {
		return TextChunkMessage{}, fmt.Errorf("failed to parse chunk ID: %w", err)
	}

	var dbResponse NotionDatabase
	if err := fetchNotionResource(ctx, client, fmt.Sprintf("https://api.notion.com/v1/databases/%s", metadata.ResourceID), &dbResponse); err != nil {
		return TextChunkMessage{}, fmt.Errorf("failed to fetch database: %w", err)
	}

	var blockResponse NotionPageResponse

	dbTitle := "Untitled Database"
	for _, text := range dbResponse.Title {
		dbTitle += text.PlainText
	}
	if dbTitle != "" {
		blockResponse.Results = append(blockResponse.Results, Block{
			ID:             fmt.Sprintf("%s_title", metadata.ResourceID),
			Type:           "heading_1",
			CreatedTime:    dbResponse.CreatedTime,
			LastEditedTime: dbResponse.LastEditedTime,
			Content: map[string]interface{}{
				"rich_text": []map[string]interface{}{
					{
						"type": "text",
						"text": map[string]interface{}{
							"content": dbTitle,
						},
						"plain_text": dbTitle,
					},
				},
			},
		})
	}

	if len(dbResponse.Description) > 0 {
		descriptionText := ""
		for _, text := range dbResponse.Description {
			descriptionText += text.PlainText + " "
		}
		if descriptionText != "" {
			blockResponse.Results = append(blockResponse.Results, Block{
				ID:             fmt.Sprintf("%s_description", metadata.ResourceID),
				Type:           "paragraph",
				CreatedTime:    dbResponse.CreatedTime,
				LastEditedTime: dbResponse.LastEditedTime,
				Content: map[string]interface{}{
					"rich_text": []map[string]interface{}{
						{
							"type": "text",
							"text": map[string]interface{}{
								"content": descriptionText,
							},
							"plain_text": descriptionText,
						},
					},
				},
			})
		}
	}

	queryBody := map[string]interface{}{
		"page_size": 100,
	}
	bodyBytes, err := json.Marshal(queryBody)
	if err != nil {
		return TextChunkMessage{}, fmt.Errorf("failed to marshal query body: %w", err)
	}

	queryReq, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("https://api.notion.com/v1/databases/%s/query", metadata.ResourceID), bytes.NewBuffer(bodyBytes))
	if err != nil {
		return TextChunkMessage{}, fmt.Errorf("failed to create query request: %w", err)
	}
	queryReq.Header.Set("Notion-Version", NotionVersion)
	queryReq.Header.Set("Content-Type", "application/json")

	queryResp, err := client.Do(queryReq)
	if err != nil {
		return TextChunkMessage{}, fmt.Errorf("failed to execute query request: %w", err)
	}
	defer queryResp.Body.Close()

	var dbQueryResponse NotionDatabaseResponse
	if err := json.NewDecoder(queryResp.Body).Decode(&dbQueryResponse); err != nil {
		return TextChunkMessage{}, fmt.Errorf("failed to decode query response: %w", err)
	}

	rowCounter := 0
	for _, row := range dbQueryResponse.Results {
		rowCounter++
		rowContent := ""
		properties, ok := row["properties"].(map[string]interface{})
		if !ok {
			continue
		}

		for propName, propValue := range properties {
			propObj, ok := propValue.(map[string]interface{})
			if !ok {
				continue
			}

			propType, ok := propObj["type"].(string)
			if !ok {
				continue
			}

			value := extractPropertyValue(propType, propObj)
			if value != "" {
				rowContent += fmt.Sprintf("%s: %s\n", propName, value)
			}
		}

		if rowContent != "" {
			blockResponse.Results = append(blockResponse.Results,
				injectMetadataBlock(
					fmt.Sprintf("%s_row_%d", metadata.ResourceID, rowCounter),
					rowContent,
					dbResponse.CreatedTime,
					dbResponse.LastEditedTime,
				),
			)
		}
	}

	chunker := NewNotionChunker(400, 80)
	processedBlocks := chunker.ProcessBlocks(blockResponse)

	var startBlock, endBlock *ProcessedBlock
	for i := range processedBlocks {
		if processedBlocks[i].ID == startBlockID {
			startBlock = &processedBlocks[i]
		}
		if processedBlocks[i].ID == endBlockID {
			endBlock = &processedBlocks[i]
		}
	}

	if startBlock == nil || endBlock == nil {
		return TextChunkMessage{}, fmt.Errorf("could not find start or end block")
	}

	var chunkWords []string
	for _, block := range processedBlocks {
		if block.StartOffset >= startBlock.StartOffset &&
			block.StartOffset+block.WordCount <= endBlock.StartOffset+endBlock.WordCount {
			if block.ID == startBlockID {
				if len(block.Words) > startOffset {
					chunkWords = append(chunkWords, block.Words[startOffset:]...)
				}
			} else if block.ID == endBlockID {
				if endOffset > 0 && endOffset <= len(block.Words) {
					chunkWords = append(chunkWords, block.Words[:endOffset]...)
				}
			} else {
				chunkWords = append(chunkWords, block.Words...)
			}
		}
	}

	chunk := TextChunkMessage{
		Metadata: metadata,
		Content:  strings.Join(chunkWords, " "),
	}
	return chunk, nil
}
