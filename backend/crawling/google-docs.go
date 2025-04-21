package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"
)

// DocProcessor handles processing and retrieval of Google Docs content
type DocProcessor struct {
	service         *docs.Service
	rateLimiter     *RateLimiterService
	baseChunkSize   uint64
	baseOverlapSize uint64
}

type ElementType string

const (
	ElementTypeParagraph ElementType = "paragraph"
	ElementTypeTable     ElementType = "table"
	ElementTypeHeader    ElementType = "header"
	ElementTypeFooter    ElementType = "footer"
	ElementTypeList      ElementType = "list"
)

// Position stores the exact position of content within a document
type Position struct {
	ElementType ElementType
	SectionID   string
	ParaIndex   int
	TableIndex  int
	RowIndex    int
	CellIndex   int
	Offset      int
}

// WordInfo stores word position information efficiently
type WordInfo struct {
	Position Position
	Word     string
	Length   int
}

// NewDocProcessor initializes a new DocProcessor with a Google Docs service
func NewDocProcessor(ctx context.Context, client *http.Client, rateLimiter *RateLimiterService) (*DocProcessor, error) {
	srv, err := docs.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("failed to create Docs service: %w", err)
	}

	return &DocProcessor{
		service:         srv,
		rateLimiter:     rateLimiter,
		baseChunkSize:   400,
		baseOverlapSize: 80,
	}, nil
}

// Process chunks a Google Doc into overlapping segments
func (dp *DocProcessor) DocsProcess(ctx context.Context, file File) (File, error) {
	if len(file.File) == 0 {
		return file, nil
	}

	metadata := file.File[0].Metadata
	if err := dp.DocsValidate(ctx, metadata.UserID); err != nil {
		return file, err
	}

	doc, err := dp.DocsFetchDocument(ctx, metadata.ResourceID)
	if err != nil {
		return file, err
	}

	chunks, err := dp.ChunkDocument(doc, metadata)
	if err != nil {
		return file, err
	}
	return File{File: chunks}, nil
}

// Retrieve fetches a specific chunk from a Google Doc based on its ChunkID
func (dp *DocProcessor) DocsRetrieve(ctx context.Context, metadata Metadata) (TextChunkMessage, error) {
	if err := dp.DocsValidate(ctx, metadata.UserID); err != nil {
		return TextChunkMessage{}, err
	}

	doc, err := dp.DocsFetchDocument(ctx, metadata.ResourceID)
	if err != nil {
		return TextChunkMessage{}, err
	}

	startPos, endPos, err := dp.ParseDocsChunkID(metadata.ChunkID)
	if err != nil {
		return TextChunkMessage{}, err
	}

	chunkWords, err := dp.ExtractDocsChunk(doc, startPos, endPos)
	if err != nil {
		return TextChunkMessage{}, err
	}

	result := TextChunkMessage{
		Metadata: metadata,
		Content:  strings.Join(chunkWords, " "),
	}
	return result, nil
}

// validate ensures the userID is present and respects rate limits
func (dp *DocProcessor) DocsValidate(ctx context.Context, userID string) error {
	if userID == "" {
		return fmt.Errorf("userID required for per-user rate limiting")
	}

	if err := dp.rateLimiter.Wait(ctx, "GOOGLE_DOCS", userID); err != nil {
		return fmt.Errorf("rate limit wait failed: %w", err)
	}

	return nil
}

// fetchDocument retrieves a Google Doc by its resource ID
func (dp *DocProcessor) DocsFetchDocument(ctx context.Context, resourceID string) (*docs.Document, error) {
	doc, err := dp.service.Documents.Get(resourceID).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	return doc, nil
}

// extractAllWords extracts all words from a document with their positions
func (dp *DocProcessor) extractAllWords(doc *docs.Document) ([]WordInfo, error) {
	wordInfoList := make([]WordInfo, 0, 5000)
	wordInfoList = dp.extractWordsFromBody(doc.Body.Content, wordInfoList)

	for headerID, header := range doc.Headers {
		position := Position{
			ElementType: ElementTypeHeader,
			SectionID:   headerID,
		}
		wordInfoList = dp.extractWordsFromElements(header.Content, position, wordInfoList)
	}

	for footerID, footer := range doc.Footers {
		position := Position{
			ElementType: ElementTypeFooter,
			SectionID:   footerID,
		}
		wordInfoList = dp.extractWordsFromElements(footer.Content, position, wordInfoList)
	}

	return wordInfoList, nil
}

// extractWordsFromBody extracts words from the main document body
func (dp *DocProcessor) extractWordsFromBody(elements []*docs.StructuralElement, wordInfoList []WordInfo) []WordInfo {
	for elementIndex, elem := range elements {
		if elem.Paragraph != nil {
			position := Position{
				ElementType: ElementTypeParagraph,
				SectionID:   "main",
				ParaIndex:   elementIndex,
			}

			if elem.Paragraph.Bullet != nil {
				position.ElementType = ElementTypeList
			}

			wordInfoList = dp.extractWordsFromParagraph(elem.Paragraph, position, wordInfoList)
		}

		if elem.Table != nil {
			position := Position{
				ElementType: ElementTypeTable,
				SectionID:   "main",
				TableIndex:  elementIndex,
			}

			for rowIndex, row := range elem.Table.TableRows {
				position.RowIndex = rowIndex

				for cellIndex, cell := range row.TableCells {
					position.CellIndex = cellIndex
					wordInfoList = dp.extractWordsFromElements(cell.Content, position, wordInfoList)
				}
			}
		}

		if elem.SectionBreak != nil {
		}
	}

	return wordInfoList
}

// extractWordsFromElements processes a collection of structural elements
func (dp *DocProcessor) extractWordsFromElements(elements []*docs.StructuralElement, basePosition Position, wordInfoList []WordInfo) []WordInfo {
	for elementIndex, elem := range elements {
		if elem.Paragraph != nil {
			position := basePosition
			position.ParaIndex = elementIndex
			wordInfoList = dp.extractWordsFromParagraph(elem.Paragraph, position, wordInfoList)
		}

		if elem.Table != nil {
			position := basePosition
			position.TableIndex = elementIndex

			for rowIndex, row := range elem.Table.TableRows {
				position.RowIndex = rowIndex

				for cellIndex, cell := range row.TableCells {
					position.CellIndex = cellIndex
					wordInfoList = dp.extractWordsFromElements(cell.Content, position, wordInfoList)
				}
			}
		}
	}

	return wordInfoList
}

// extractWordsFromParagraph extracts words from a paragraph element
func (dp *DocProcessor) extractWordsFromParagraph(paragraph *docs.Paragraph, position Position, wordInfoList []WordInfo) []WordInfo {
	offset := 0

	for _, textElem := range paragraph.Elements {
		if textElem.TextRun == nil {
			continue
		}

		content := strings.NewReplacer("\n", " ", "\r", " ").Replace(textElem.TextRun.Content)
		words := strings.Fields(content)

		for _, word := range words {
			currentPosition := position
			currentPosition.Offset = offset

			wordInfoList = append(wordInfoList, WordInfo{
				Position: currentPosition,
				Word:     word,
				Length:   len(word),
			})

			offset += len(word) + 1
		}
	}

	return wordInfoList
}

// chunkDocument splits a document into overlapping chunks
func (dp *DocProcessor) ChunkDocument(doc *docs.Document, baseMetadata Metadata) ([]TextChunkMessage, error) {
	var chunks []TextChunkMessage

	wordInfoList, err := dp.extractAllWords(doc)
	if err != nil {
		return nil, err
	}

	totalWords := len(wordInfoList)
	for startIndex := 0; startIndex < totalWords; startIndex += int(dp.baseChunkSize) - int(dp.baseOverlapSize) {
		endIndex := startIndex + int(dp.baseChunkSize)

		if endIndex > totalWords {
			endIndex = totalWords
		}

		if startIndex > 0 && endIndex-startIndex < int(dp.baseOverlapSize) {
			continue
		}

		chunkWords := make([]string, endIndex-startIndex)
		for i := 0; i < endIndex-startIndex; i++ {
			chunkWords[i] = wordInfoList[startIndex+i].Word
		}

		startInfo := wordInfoList[startIndex]
		endInfo := wordInfoList[endIndex-1]

		chunk, err := dp.createDocsChunk(chunkWords, baseMetadata, startInfo.Position, endInfo.Position)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

// createChunk constructs a TextChunkMessage with metadata
func (dp *DocProcessor) createDocsChunk(words []string, baseMetadata Metadata, startPos, endPos Position) (TextChunkMessage, error) {
	chunkMetadata := baseMetadata
	chunkMetadata.ChunkID = dp.formatPositionToChunkID(startPos, endPos)

	return TextChunkMessage{
		Metadata: chunkMetadata,
		Content:  strings.Join(words, " "),
	}, nil
}

// formatPositionToChunkID converts start and end positions to a formatted chunk ID
func (dp *DocProcessor) formatPositionToChunkID(startPos, endPos Position) string {
	return fmt.Sprintf("Start{Type:%s,Section:%s,Para:%d,Table:%d,Row:%d,Cell:%d,Offset:%d}-End{Type:%s,Section:%s,Para:%d,Table:%d,Row:%d,Cell:%d,Offset:%d}",
		startPos.ElementType, startPos.SectionID, startPos.ParaIndex, startPos.TableIndex, startPos.RowIndex, startPos.CellIndex, startPos.Offset,
		endPos.ElementType, endPos.SectionID, endPos.ParaIndex, endPos.TableIndex, endPos.RowIndex, endPos.CellIndex, endPos.Offset)
}

// parseChunkID extracts chunk boundaries from the ChunkID string
func (dp *DocProcessor) ParseDocsChunkID(chunkID string) (startPos, endPos Position, err error) {
	parts := strings.Split(chunkID, "-End{")
	if len(parts) != 2 {
		return Position{}, Position{}, fmt.Errorf("invalid ChunkID format: missing End section")
	}

	startPart := strings.TrimPrefix(parts[0], "Start{")
	startPart = strings.TrimSuffix(startPart, "}")
	startFields := strings.Split(startPart, ",")
	if len(startFields) != 7 {
		return Position{}, Position{}, fmt.Errorf("invalid start position format: expected 7 fields, got %d", len(startFields))
	}

	endPart := strings.TrimPrefix(parts[1], "End{")
	endPart = strings.TrimSuffix(endPart, "}")
	endFields := strings.Split(endPart, ",")
	if len(endFields) != 7 {
		return Position{}, Position{}, fmt.Errorf("invalid end position format: expected 7 fields, got %d", len(endFields))
	}

	startType := strings.TrimPrefix(startFields[0], "Type:")
	startSection := strings.TrimPrefix(startFields[1], "Section:")
	startPos.ParaIndex, _ = strconv.Atoi(strings.TrimPrefix(startFields[2], "Para:"))
	startPos.TableIndex, _ = strconv.Atoi(strings.TrimPrefix(startFields[3], "Table:"))
	startPos.RowIndex, _ = strconv.Atoi(strings.TrimPrefix(startFields[4], "Row:"))
	startPos.CellIndex, _ = strconv.Atoi(strings.TrimPrefix(startFields[5], "Cell:"))
	startPos.Offset, _ = strconv.Atoi(strings.TrimPrefix(startFields[6], "Offset:"))
	startPos.ElementType = ElementType(startType)
	startPos.SectionID = startSection

	endType := strings.TrimPrefix(endFields[0], "Type:")
	endSection := strings.TrimPrefix(endFields[1], "Section:")
	endPos.ParaIndex, _ = strconv.Atoi(strings.TrimPrefix(endFields[2], "Para:"))
	endPos.TableIndex, _ = strconv.Atoi(strings.TrimPrefix(endFields[3], "Table:"))
	endPos.RowIndex, _ = strconv.Atoi(strings.TrimPrefix(endFields[4], "Row:"))
	endPos.CellIndex, _ = strconv.Atoi(strings.TrimPrefix(endFields[5], "Cell:"))
	endPos.Offset, _ = strconv.Atoi(strings.TrimPrefix(endFields[6], "Offset:"))
	endPos.ElementType = ElementType(endType)
	endPos.SectionID = endSection

	startPos.ElementType = ElementType(startType)
	endPos.ElementType = ElementType(endType)

	startPos.SectionID = startSection
	endPos.SectionID = endSection

	return startPos, endPos, nil
}

// ExtractDocsChunk extracts the content between two positions in a document
func (dp *DocProcessor) ExtractDocsChunk(doc *docs.Document, startPos, endPos Position) ([]string, error) {
	var chunkWords []string

	allWords, err := dp.extractAllWords(doc)
	if err != nil {
		return nil, err
	}

	for _, wordInfo := range allWords {
		if wordInfo.Position.InRange(startPos, endPos) {
			chunkWords = append(chunkWords, wordInfo.Word)
		}
	}
	if len(chunkWords) == 0 {
		return nil, fmt.Errorf("no content found between specified positions")
	}
	return chunkWords, nil
}

// InRange checks if this position is within the range defined by start and end positions
func (pos Position) InRange(startPos, endPos Position) bool {
	afterStart := !pos.Before(startPos) ||
		(pos.ElementType == startPos.ElementType &&
			pos.SectionID == startPos.SectionID &&
			pos.ParaIndex == startPos.ParaIndex &&
			pos.TableIndex == startPos.TableIndex &&
			pos.RowIndex == startPos.RowIndex &&
			pos.CellIndex == startPos.CellIndex &&
			pos.Offset >= startPos.Offset)

	beforeEnd := pos.Before(endPos) ||
		(pos.ElementType == endPos.ElementType &&
			pos.SectionID == endPos.SectionID &&
			pos.ParaIndex == endPos.ParaIndex &&
			pos.TableIndex == endPos.TableIndex &&
			pos.RowIndex == endPos.RowIndex &&
			pos.CellIndex == endPos.CellIndex &&
			pos.Offset <= endPos.Offset)

	return afterStart && beforeEnd
}

// Before determines if this position comes before another position in the document
func (a Position) Before(b Position) bool {
	if a.ElementType != b.ElementType {
		if a.ElementType == ElementTypeHeader && (b.ElementType != ElementTypeHeader) {
			return true
		}
		if a.ElementType == ElementTypeFooter && (b.ElementType != ElementTypeFooter) {
			return false
		}
		if b.ElementType == ElementTypeFooter && (a.ElementType != ElementTypeFooter) {
			return true
		}
		if b.ElementType == ElementTypeHeader && (a.ElementType != ElementTypeHeader) {
			return false
		}
	}

	if a.SectionID != b.SectionID {
		return a.SectionID < b.SectionID
	}

	if (a.ElementType == ElementTypeTable) != (b.ElementType == ElementTypeTable) {
		return a.ElementType != ElementTypeTable
	}

	if a.ElementType == ElementTypeTable && b.ElementType == ElementTypeTable {
		if a.TableIndex != b.TableIndex {
			return a.TableIndex < b.TableIndex
		}
		if a.RowIndex != b.RowIndex {
			return a.RowIndex < b.RowIndex
		}
		if a.CellIndex != b.CellIndex {
			return a.CellIndex < b.CellIndex
		}
		if a.ParaIndex != b.ParaIndex {
			return a.ParaIndex < b.ParaIndex
		}
	} else {
		if a.ParaIndex != b.ParaIndex {
			return a.ParaIndex < b.ParaIndex
		}
	}

	return a.Offset < b.Offset
}

// ProcessGoogleDoc is used to process a Google Doc
func ProcessGoogleDoc(ctx context.Context, client *http.Client, file File) (File, error) {
	processor, err := NewDocProcessor(ctx, client, rateLimiter)
	if err != nil {
		return file, err
	}
	return processor.DocsProcess(ctx, file)
}

// RetrieveGoogleDoc is used to retrieve a chunk of a Google Doc
func RetrieveGoogleDoc(ctx context.Context, client *http.Client, metadata Metadata) (TextChunkMessage, error) {
	processor, err := NewDocProcessor(ctx, client, rateLimiter)
	if err != nil {
		return TextChunkMessage{}, err
	}
	return processor.DocsRetrieve(ctx, metadata)
}
