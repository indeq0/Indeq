package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dslipak/pdf"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type PdfProcessor struct {
	service         *drive.Service
	rateLimiter     *RateLimiterService
	baseChunkSize   uint64
	baseOverlapSize uint64
}

// NewPdfProcessor initializes a new PdfProcessor with a Google Docs service
func NewPdfProcessor(ctx context.Context, client *http.Client, rateLimiter *RateLimiterService) (*PdfProcessor, error) {
	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("failed to create Drive service: %w", err)
	}

	return &PdfProcessor{
		service:         srv,
		rateLimiter:     rateLimiter,
		baseChunkSize:   400,
		baseOverlapSize: 80,
	}, nil
}

// Process chunks a Google Doc into overlapping segments
func (p *PdfProcessor) PdfProcess(ctx context.Context, file File) (File, error) {
	if len(file.File) == 0 {
		return file, nil
	}

	metadata := file.File[0].Metadata

	// Create a temporary file with a unique name
	tempFile, err := os.CreateTemp("", fmt.Sprintf("pdf-process-%s-*.pdf", metadata.ResourceID))
	if err != nil {
		return file, fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	tempFile.Close()
	defer os.Remove(tempPath)

	err = p.downloadPdf(metadata.ResourceID, tempPath)
	if err != nil {
		return file, err
	}
	text, err := extractText(tempPath)
	if err != nil {
		return file, err
	}

	chunks, err := p.ChunkText(text, metadata)
	if err != nil {
		return file, err
	}
	return File{File: chunks}, nil
}

// PdfRetrieve retrieves a specific chunk from a Google PDF file
func (p *PdfProcessor) PdfRetrieve(ctx context.Context, metadata Metadata) (TextChunkMessage, error) {
	// Create a temporary file with a unique name
	tempFile, err := os.CreateTemp("", fmt.Sprintf("pdf-retrieve-%s-*.pdf", metadata.ResourceID))
	if err != nil {
		return TextChunkMessage{}, fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	tempFile.Close()          // Close immediately as we'll reopen it in downloadPdf
	defer os.Remove(tempPath) // Clean up the temp file when done

	err = p.downloadPdf(metadata.ResourceID, tempPath)
	if err != nil {
		return TextChunkMessage{}, err
	}

	text, err := extractText(tempPath)
	if err != nil {
		return TextChunkMessage{}, err
	}
	chunks, err := p.RetrievePdfChunk(ctx, metadata.ChunkID, text)
	if err != nil {
		return TextChunkMessage{}, err
	}

	return TextChunkMessage{
		Metadata: metadata,
		Content:  chunks,
	}, nil
}

// downloadPdf downloads a PDF file from Google Drive and saves it to a local file
func (p *PdfProcessor) downloadPdf(ResourceID string, outputPath string) error {

	resp, err := p.service.Files.Get(ResourceID).Download()
	if err != nil {
		return fmt.Errorf("failed to download file: %v", err)
	}
	defer resp.Body.Close()

	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %v", err)
	}
	defer outFile.Close()

	if _, err = io.Copy(outFile, resp.Body); err != nil {
		return fmt.Errorf("failed to save PDF: %v", err)
	}
	return nil
}

func sanitizeText(input string) string {
	if utf8.ValidString(input) {
		return input
	}

	bytes := []byte(input)
	valid := make([]byte, 0, len(bytes))

	for len(bytes) > 0 {
		r, size := utf8.DecodeRune(bytes)
		if r == utf8.RuneError {
			bytes = bytes[1:]
			continue
		}
		valid = append(valid, bytes[:size]...)
		bytes = bytes[size:]
	}

	return string(valid)
}

// extractText extracts text from a PDF file
func extractText(path string) (string, error) {
	f, err := pdf.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open PDF: %v", err)
	}

	var buf bytes.Buffer
	text, err := f.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("text extraction failed: %v", err)
	}

	buf.ReadFrom(text)
	return sanitizeText(buf.String()), nil
}

func (p *PdfProcessor) ChunkText(text string, metadata Metadata) ([]TextChunkMessage, error) {
	words := strings.Fields(text)
	totalWords := len(words)
	if totalWords == 0 {
		return nil, fmt.Errorf("no words found in text")
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

		metadata.ChunkID = fmt.Sprintf("%d-%d", start, end-1)

		fileChunks = append(fileChunks, TextChunkMessage{
			Metadata: metadata,
			Content:  chunkText,
		})
	}

	return fileChunks, nil
}

func (p *PdfProcessor) RetrievePdfChunk(ctx context.Context, chunkID string, text string) (string, error) {
	parts := strings.Split(chunkID, "-")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid chunk ID format")
	}

	startoffset, err := strconv.Atoi(parts[0])
	if err != nil {
		return "", fmt.Errorf("failed to convert startoffset to int: %v", err)
	}
	endoffset, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("failed to convert endoffset to int: %v", err)
	}

	textLen := len(text)
	if startoffset < 0 {
		startoffset = 0
	}
	if endoffset > textLen {
		endoffset = textLen
	}
	if startoffset > endoffset {
		return "", fmt.Errorf("invalid offset range")
	}

	chunkText := text[startoffset:endoffset]
	return sanitizeText(chunkText), nil
}

// ProcessGooglePDF processes a Google PDF file into chunks
func ProcessGooglePDF(ctx context.Context, client *http.Client, file File) (File, error) {
	pdfProcessor, err := NewPdfProcessor(ctx, client, rateLimiter)
	if err != nil {
		return File{}, fmt.Errorf("failed to create PDF processor: %w", err)
	}
	return pdfProcessor.PdfProcess(ctx, file)
}

// RetrieveGooglePDF retrieves a specific chunk from a Google PDF file
func RetrieveGooglePDF(ctx context.Context, client *http.Client, metadata Metadata) (TextChunkMessage, error) {
	processor, err := NewPdfProcessor(ctx, client, rateLimiter)
	if err != nil {
		return TextChunkMessage{}, err
	}
	return processor.PdfRetrieve(ctx, metadata)
}
