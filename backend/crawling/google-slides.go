package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"google.golang.org/api/option"
	"google.golang.org/api/slides/v1"
)

// SlidesProcessor handles processing and retrieval of Google Slides content
type SlidesProcessor struct {
	service         *slides.Service
	rateLimiter     *RateLimiterService
	baseChunkSize   uint64
	baseOverlapSize uint64
}

// SlideElementType represents different types of content in a slide
type SlideElementType string

const (
	SlideElementTypeText  SlideElementType = "text"
	SlideElementTypeTable SlideElementType = "table"
	//SlideElementTypeImage SlideElementType = "image"
	//SlideElementTypeVideo SlideElementType = "video"
	SlideElementTypeChart SlideElementType = "chart"
	SlideElementTypeShape SlideElementType = "shape"
)

// SlidePosition stores the exact position of content within a slide
type SlidePosition struct {
	ElementType  SlideElementType
	SlideIndex   int
	GroupIndex   int
	ElementIndex int
	Offset       int
}

// SlideWordInfo stores word position information efficiently
type SlideWordInfo struct {
	Position SlidePosition
	Word     string
	Length   int
}

// NewSlidesProcessor initializes a new SlidesProcessor with a Google Slides service
func NewSlidesProcessor(ctx context.Context, client *http.Client, rateLimiter *RateLimiterService) (*SlidesProcessor, error) {
	srv, err := slides.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("failed to create Slides service: %w", err)
	}

	return &SlidesProcessor{
		service:         srv,
		rateLimiter:     rateLimiter,
		baseChunkSize:   400,
		baseOverlapSize: 80,
	}, nil
}

// ProcessGoogleSlides processes a Google Slides presentation
func (sp *SlidesProcessor) SlidesProcess(ctx context.Context, file File) (File, error) {
	if len(file.File) == 0 {
		return file, nil
	}

	metadata := file.File[0].Metadata
	if err := sp.SlidesValidate(ctx, metadata.UserID); err != nil {
		return file, err
	}
	doc, err := sp.SlidesFetchDocument(ctx, metadata.ResourceID)
	if err != nil {
		return file, err
	}

	chunks, err := sp.ChunkPresentation(doc, metadata)
	if err != nil {
		return file, err
	}
	return File{File: chunks}, nil
}

// Retrieve fetches a specific chunk from a Google Slides presentation based on its ChunkID
func (sp *SlidesProcessor) SlidesRetrieve(ctx context.Context, metadata Metadata, chunkIDs []string) ([]TextChunkMessage, error) {
	if err := sp.SlidesValidate(ctx, metadata.UserID); err != nil {
		return nil, err
	}

	doc, err := sp.SlidesFetchDocument(ctx, metadata.ResourceID)
	if err != nil {
		return nil, err
	}

	results := make([]TextChunkMessage, 0, len(chunkIDs))
	for _, chunkID := range chunkIDs {
		startSlide, startElement, startOffset, endSlide, endElement, endOffset, err := sp.ParseSlidesChunkID(chunkID)
		if err != nil {
			return nil, err
		}

		chunkWords, err := sp.ExtractSlidesChunk(doc, startSlide, startElement, startOffset, endSlide, endElement, endOffset)
		if err != nil {
			return nil, err
		}

		result := TextChunkMessage{
			Metadata: metadata,
			Content:  strings.Join(chunkWords, " "),
		}
		results = append(results, result)
	}
	return results, nil
}

// validate ensures the userID is present and respects rate limits
func (sp *SlidesProcessor) SlidesValidate(ctx context.Context, userID string) error {
	if userID == "" {
		return fmt.Errorf("userID required for per-user rate limiting")
	}
	if err := sp.rateLimiter.Wait(ctx, "GOOGLE_SLIDES", userID); err != nil {
		return fmt.Errorf("rate limit wait failed: %w", err)
	}

	return nil
}

// fetchDocument retrieves a Google Slides presentation by its resource ID
func (sp *SlidesProcessor) SlidesFetchDocument(ctx context.Context, resourceID string) (*slides.Presentation, error) {
	presentation, err := sp.service.Presentations.Get(resourceID).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get presentation: %w", err)
	}

	return presentation, nil
}

// extractAllWords extracts all words from a presentation with their positions
func (sp *SlidesProcessor) extractAllWords(presentation *slides.Presentation) ([]SlideWordInfo, error) {
	wordInfoList := make([]SlideWordInfo, 0, 5000)

	for slideIndex, slide := range presentation.Slides {
		wordInfoList = sp.extractWordsFromSlide(slide, slideIndex, wordInfoList)
	}

	return wordInfoList, nil
}

// extractWordsFromSlide processes a single slide and extracts words from all its elements
func (sp *SlidesProcessor) extractWordsFromSlide(slide *slides.Page, slideIndex int, wordInfoList []SlideWordInfo) []SlideWordInfo {
	for elementIndex, element := range slide.PageElements {
		switch {
		case element.Shape != nil:
			wordInfoList = sp.extractWordsFromShape(element.Shape, slideIndex, elementIndex, wordInfoList)
		case element.Table != nil:
			wordInfoList = sp.extractWordsFromTable(element.Table, slideIndex, elementIndex, wordInfoList)
			// case element.Image != nil:
			// 	if element.Image.ContentUrl != "" {
			// 		wordInfoList = append(wordInfoList, SlideWordInfo{
			// 			Position: SlidePosition{
			// 				ElementType:  SlideElementTypeImage,
			// 				SlideIndex:   slideIndex,
			// 				ElementIndex: elementIndex,
			// 				Offset:       0,
			// 			},
			// 			Word:   element.Image.ContentUrl,
			// 			Length: len(element.Image.ContentUrl),
			// 		})
			// 	}
			// case element.Video != nil:
			// 	if element.Video.Url != "" {
			// 		wordInfoList = append(wordInfoList, SlideWordInfo{
			// 			Position: SlidePosition{
			// 				ElementType:  SlideElementTypeVideo,
			// 				SlideIndex:   slideIndex,
			// 				ElementIndex: elementIndex,
			// 				Offset:       0,
			// 			},
			// 			Word:   element.Video.Url,
			// 			Length: len(element.Video.Url),
			// 		})
			// 	}
		}
	}

	return wordInfoList
}

// extractWordsFromShape processes a shape element and extracts its text content
func (sp *SlidesProcessor) extractWordsFromShape(shape *slides.Shape, slideIndex, elementIndex int, wordInfoList []SlideWordInfo) []SlideWordInfo {
	if shape.Text == nil {
		return wordInfoList
	}

	offset := 0
	for _, textElem := range shape.Text.TextElements {
		if textElem.TextRun == nil {
			continue
		}
		content := strings.NewReplacer("\n", " ", "\r", " ").Replace(textElem.TextRun.Content)
		words := strings.Fields(content)

		for _, word := range words {
			wordInfoList = append(wordInfoList, SlideWordInfo{
				Position: SlidePosition{
					ElementType:  SlideElementTypeShape,
					SlideIndex:   slideIndex,
					ElementIndex: elementIndex,
					Offset:       offset,
				},
				Word:   word,
				Length: len(word),
			})
			offset += len(word) + 1
		}
	}
	return wordInfoList
}

// extractWordsFromTable processes a table element and extracts text from all cells
func (sp *SlidesProcessor) extractWordsFromTable(table *slides.Table, slideIndex, elementIndex int, wordInfoList []SlideWordInfo) []SlideWordInfo {
	for rowIndex, row := range table.TableRows {
		for cellIndex, cell := range row.TableCells {
			offset := 0
			for _, textElem := range cell.Text.TextElements {
				if textElem.TextRun == nil {
					continue
				}

				content := strings.NewReplacer("\n", " ", "\r", " ").Replace(textElem.TextRun.Content)
				words := strings.Fields(content)

				for _, word := range words {
					wordInfoList = append(wordInfoList, SlideWordInfo{
						Position: SlidePosition{
							ElementType:  SlideElementTypeTable,
							SlideIndex:   slideIndex,
							ElementIndex: elementIndex,
							GroupIndex:   rowIndex*len(row.TableCells) + cellIndex,
							Offset:       offset,
						},
						Word:   word,
						Length: len(word),
					})
					offset += len(word) + 1
				}
			}
		}
	}
	return wordInfoList
}

// ChunkPresentation splits a Google Slides presentation into text chunks
func (sp *SlidesProcessor) ChunkPresentation(presentation *slides.Presentation, baseMetadata Metadata) ([]TextChunkMessage, error) {
	var chunks []TextChunkMessage

	wordInfoList, err := sp.extractAllWords(presentation)
	if err != nil {
		return nil, err
	}

	totalWords := len(wordInfoList)
	for startIndex := 0; startIndex < totalWords; startIndex += int(sp.baseChunkSize) - int(sp.baseOverlapSize) {
		endIndex := startIndex + int(sp.baseChunkSize)
		if endIndex > totalWords {
			endIndex = totalWords
		}

		if startIndex > 0 && endIndex-startIndex < int(sp.baseOverlapSize) {
			continue
		}

		chunkWords := make([]string, endIndex-startIndex)
		for i := 0; i < endIndex-startIndex; i++ {
			chunkWords[i] = wordInfoList[startIndex+i].Word
		}

		startInfo := wordInfoList[startIndex]
		endInfo := wordInfoList[endIndex-1]
		endOffset := endInfo.Position.Offset + endInfo.Length

		chunk, err := sp.CreateSlidesChunk(chunkWords, baseMetadata, startInfo.Position.SlideIndex, startInfo.Position.ElementIndex, startInfo.Position.Offset, endInfo.Position.SlideIndex, endInfo.Position.ElementIndex, endOffset)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// parseChunkID extracts chunk boundaries from the ChunkID string
func (sp *SlidesProcessor) ParseSlidesChunkID(chunkID string) (startSlide, startElement, startOffset, endSlide, endElement, endOffset int, err error) {
	parts := strings.Split(chunkID, "-End{")
	if len(parts) != 2 {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("invalid ChunkID format: missing End section")
	}

	startPart := strings.TrimPrefix(parts[0], "Start{")
	startPart = strings.TrimSuffix(startPart, "}")
	startFields := strings.Split(startPart, ",")
	if len(startFields) != 4 {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("invalid start position format: expected 4 fields, got %d", len(startFields))
	}

	endPart := strings.TrimPrefix(parts[1], "End{")
	endPart = strings.TrimSuffix(endPart, "}")
	endFields := strings.Split(endPart, ",")
	if len(endFields) != 4 {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("invalid end position format: expected 4 fields, got %d", len(endFields))
	}

	startType := strings.TrimPrefix(startFields[0], "Type:")
	startSlide, _ = strconv.Atoi(strings.TrimPrefix(startFields[1], "Slide:"))
	startElement, _ = strconv.Atoi(strings.TrimPrefix(startFields[2], "Element:"))
	startOffset, _ = strconv.Atoi(strings.TrimPrefix(startFields[3], "Offset:"))

	endType := strings.TrimPrefix(endFields[0], "Type:")
	endSlide, _ = strconv.Atoi(strings.TrimPrefix(endFields[1], "Slide:"))
	endElement, _ = strconv.Atoi(strings.TrimPrefix(endFields[2], "Element:"))
	endOffset, _ = strconv.Atoi(strings.TrimPrefix(endFields[3], "Offset:"))

	if startType != string(SlideElementTypeShape) || endType != string(SlideElementTypeShape) {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("unsupported element type: start=%s, end=%s", startType, endType)
	}

	if startSlide > endSlide {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("invalid slide range: start=%d, end=%d", startSlide, endSlide)
	}

	if startSlide == endSlide && startElement > endElement {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("invalid element range within slide %d: start=%d, end=%d", startSlide, startElement, endElement)
	}

	return startSlide, startElement, startOffset, endSlide, endElement, endOffset, nil
}

// extractSlidesChunk retrieves words for a specific chunk based on slide and offset boundaries
func (sp *SlidesProcessor) ExtractSlidesChunk(presentation *slides.Presentation, startSlide, startElement, startOffset, endSlide, endElement, endOffset int) ([]string, error) {
	var chunkWords []string

	slideMap := make(map[int]*slides.Page, len(presentation.Slides))
	for idx, slide := range presentation.Slides {
		slideMap[idx] = slide
	}

	for slideIdx := startSlide; slideIdx <= endSlide; slideIdx++ {
		slide, exists := slideMap[slideIdx]
		if !exists {
			log.Printf("Warning: Slide %d not found\n", slideIdx)
			continue
		}
		slideWords := make([]string, 0, 100)
		slideOffsets := make([]int, 0, 100)
		slideOffset := 0
		slideWordCount := 0

		for elementIndex, element := range slide.PageElements {
			if slideIdx == startSlide && elementIndex < startElement {
				continue
			}
			if slideIdx == endSlide && elementIndex > endElement {
				continue
			}

			var wordCount int

			switch {
			case element.Shape != nil && element.Shape.Text != nil:
				for _, textElem := range element.Shape.Text.TextElements {
					if textElem.TextRun == nil {
						continue
					}

					content := strings.NewReplacer("\n", " ", "\r", " ").Replace(textElem.TextRun.Content)
					words := strings.Fields(content)
					wordCount += len(words)

					for _, word := range words {
						slideWords = append(slideWords, word)
						slideOffsets = append(slideOffsets, slideOffset)
						slideOffset += len(word) + 1
					}
				}
			case element.Table != nil:
				for _, row := range element.Table.TableRows {
					for _, cell := range row.TableCells {
						for _, textElem := range cell.Text.TextElements {
							if textElem.TextRun == nil {
								continue
							}

							content := strings.NewReplacer("\n", " ", "\r", " ").Replace(textElem.TextRun.Content)
							words := strings.Fields(content)
							wordCount += len(words)

							for _, word := range words {
								slideWords = append(slideWords, word)
								slideOffsets = append(slideOffsets, slideOffset)
								slideOffset += len(word) + 1
							}
						}
					}
				}
			// case element.Image != nil && element.Image.ContentUrl != "":
			// 	slideWords = append(slideWords, element.Image.ContentUrl)
			// 	slideOffsets = append(slideOffsets, slideOffset)
			// 	slideOffset += len(element.Image.ContentUrl) + 1
			// 	wordCount = 1
			// case element.Video != nil && element.Video.Url != "":
			// 	slideWords = append(slideWords, element.Video.Url)
			// 	slideOffsets = append(slideOffsets, slideOffset)
			// 	slideOffset += len(element.Video.Url) + 1
			// 	wordCount = 1
			default:
				continue
			}
			slideWordCount += wordCount
		}

		wordsAdded := 0
		for i, offset := range slideOffsets {
			if i >= len(slideWords) {
				break
			}

			inChunk := false
			if slideIdx == startSlide && slideIdx == endSlide {
				inChunk = offset >= startOffset && offset < endOffset
			} else if slideIdx == startSlide {
				inChunk = offset >= startOffset
			} else if slideIdx == endSlide {
				inChunk = offset < endOffset
			} else {
				inChunk = true
			}

			if inChunk {
				chunkWords = append(chunkWords, slideWords[i])
				wordsAdded++
			}
		}
	}

	return chunkWords, nil
}

// CreateSlidesChunk constructs a TextChunkMessage with metadata
func (sp *SlidesProcessor) CreateSlidesChunk(words []string, baseMetadata Metadata, startSlide, startElement, startOffset, endSlide, endElement, endOffset int) (TextChunkMessage, error) {
	chunkMetadata := baseMetadata
	chunkMetadata.ChunkID = fmt.Sprintf("Start{Type:%s,Slide:%d,Element:%d,Offset:%d}-End{Type:%s,Slide:%d,Element:%d,Offset:%d}",
		SlideElementTypeShape, startSlide, startElement, startOffset,
		SlideElementTypeShape, endSlide, endElement, endOffset)

	return TextChunkMessage{
		Metadata: chunkMetadata,
		Content:  strings.Join(words, " "),
	}, nil
}

// ProcessGoogleSlides processes a Google Slides presentation into chucks
func ProcessGoogleSlides(ctx context.Context, client *http.Client, file File) (File, error) {
	processor, err := NewSlidesProcessor(ctx, client, rateLimiter)
	if err != nil {
		return file, err
	}
	return processor.SlidesProcess(ctx, file)
}

// RetrieveGoogleSlides retrieves a specific chunk from a Google Slides presentation
func RetrieveGoogleSlides(ctx context.Context, client *http.Client, metadata Metadata, chunkIDs []string) ([]TextChunkMessage, error) {
	processor, err := NewSlidesProcessor(ctx, client, rateLimiter)
	if err != nil {
		return nil, err
	}
	return processor.SlidesRetrieve(ctx, metadata, chunkIDs)
}
