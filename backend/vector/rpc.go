package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	pb "github.com/cc-0000/indeq/common/api"

	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// rpc(context, file delete request)
//   - takes in a list of files and either deletes them (exclusive = false) or deletes all files not within that list (exclusive = true)
//   - deletes for a specified user and platform.
func (s *vectorServer) DeleteFiles(ctx context.Context, req *pb.VectorFileDeleteRequest) (*pb.VectorFileDeleteResponse, error) {
	filesToKeepStr := ""
	if len(req.Files) > 0 {
		filesToKeepStr = "'" + strings.Join(req.Files, "','") + "'"
	}
	modifier := ""
	if req.Exclusive {
		modifier = "NOT"
	}
	filter := fmt.Sprintf("user_id == '%s' && platform == %d && file_path %s IN [%s]", req.UserId, req.Platform, modifier, filesToKeepStr)

	err := s.milvusClient.Delete(ctx, s.collectionName, "", filter)
	if err != nil {
		return &pb.VectorFileDeleteResponse{}, fmt.Errorf("failed to delete data: %w", err)
	}

	return &pb.VectorFileDeleteResponse{}, nil
}

// rpc (context, top k chunks request)
//   - finds the closest k vectors associated with the incoming prompt for the given user
func (s *vectorServer) GetTopKChunks(ctx context.Context, req *pb.GetTopKChunksRequest) (*pb.GetTopKChunksResponse, error) {
	embedding, err := s.embeddingClient.GenerateEmbeddings(ctx, &pb.EmbeddingRequest{
		Texts: []string{req.Prompt},
	})
	if err != nil {
		return &pb.GetTopKChunksResponse{
			TopKMetadatas: []*pb.Metadata{},
		}, err
	}

	filter := fmt.Sprintf("user_id == '%s'", req.UserId)

	outputFields := []string{"date_created", "date_modified", "file_path", "user_id", "start", "end", "title", "platform", "resource_type", "file_id", "file_url", "chunk_id", "service"}

	searchParams, err := entity.NewIndexBinFlatSearchParam(256) // Nprobe parameter (number of clusters to search through)
	if err != nil {
		return &pb.GetTopKChunksResponse{
			TopKMetadatas: []*pb.Metadata{},
		}, err
	}

	binaryVector := entity.BinaryVector(embedding.Embeddings[0])

	searchRes, err := s.milvusClient.Search(
		ctx,
		s.collectionName,
		[]string{},
		filter,
		outputFields,
		[]entity.Vector{binaryVector},
		"vector",
		entity.HAMMING,
		int(req.K),
		searchParams,
	)
	if err != nil {
		return &pb.GetTopKChunksResponse{
			TopKMetadatas: []*pb.Metadata{},
		}, err
	}

	var topKMetadatas []*pb.Metadata
	for _, result := range searchRes {

		for i := range result.ResultCount {
			// Extract field values
			dateCreated := result.Fields.GetColumn("date_created").(*entity.ColumnInt64).Data()[i]
			dateModified := result.Fields.GetColumn("date_modified").(*entity.ColumnInt64).Data()[i]
			filePath := result.Fields.GetColumn("file_path").(*entity.ColumnVarChar).Data()[i]
			userId := result.Fields.GetColumn("user_id").(*entity.ColumnVarChar).Data()[i]
			start := result.Fields.GetColumn("start").(*entity.ColumnInt64).Data()[i]
			end := result.Fields.GetColumn("end").(*entity.ColumnInt64).Data()[i]
			title := result.Fields.GetColumn("title").(*entity.ColumnVarChar).Data()[i]
			platform := result.Fields.GetColumn("platform").(*entity.ColumnInt8).Data()[i]
			fileId := result.Fields.GetColumn("file_id").(*entity.ColumnVarChar).Data()[i]
			resourceType := result.Fields.GetColumn("resource_type").(*entity.ColumnVarChar).Data()[i]
			fileUrl := result.Fields.GetColumn("file_url").(*entity.ColumnVarChar).Data()[i]
			chunkId := result.Fields.GetColumn("chunk_id").(*entity.ColumnVarChar).Data()[i]
			service := result.Fields.GetColumn("service").(*entity.ColumnVarChar).Data()[i]
			// Create TextChunkMessage
			metadata := &pb.Metadata{
				DateCreated:      timestamppb.New(time.Unix(dateCreated, 0)),
				DateLastModified: timestamppb.New(time.Unix(dateModified, 0)),
				UserId:           userId,
				FileId:           fileId,
				ResourceType:     resourceType,
				FileUrl:          fileUrl,
				FilePath:         filePath,
				Start:            uint32(start),
				End:              uint32(end),
				Title:            title,
				Platform:         pb.Platform(platform),
				ChunkId:          chunkId,
				Service:          service,
			}

			topKMetadatas = append(topKMetadatas, metadata)
		}
	}

	return &pb.GetTopKChunksResponse{
		TopKMetadatas: topKMetadatas,
	}, nil
}
