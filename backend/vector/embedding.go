package main

import (
	"context"
	"fmt"

	pb "github.com/cc-0000/indeq/common/api"
)

// func(context, batch of text chunk messages that need embedding generation)
//   - takes in a list of text chunks and sends their contents to our embedding server
//   - stores the resulting vectors and their associated metadata in our vector database
func (s *vectorServer) processBatch(ctx context.Context, batch []*pb.TextChunkMessage) error {
	// do not process empty batches
	if len(batch) == 0 {
		return nil
	}

	// turn textchunkmessages[] --> texts[]
	var texts []string
	for _, chunk := range batch {
		texts = append(texts, chunk.GetContent())
	}

	// call the embedding client to generate the embeddings
	// NOTE: expects the returned embeddings to be in the same order as sent
	embeddingRes, err := s.embeddingClient.GenerateEmbeddings(ctx, &pb.EmbeddingRequest{
		Texts: texts,
	})
	if err != nil {
		return fmt.Errorf("failed to generate embeddings for this batch: %w", err)
	}

	// store the vectors along with their embeddings in our vector database
	err = s.insertRows(ctx, batch, embeddingRes.Embeddings)
	if err != nil {
		return fmt.Errorf("failed to store embeddings in the vector database: %w", err)
	}

	return nil
}
