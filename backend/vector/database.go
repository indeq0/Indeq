package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	pb "github.com/cc-0000/indeq/common/api"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TODO: create a dynamic partition-based system for users (round robin, load balance?)
// convert 1 collection set up to multi-collection, multi-partition, hash-based binning
// we have 5 collections on the zilliz free tier
// each collection can have 64 partitions
// partitionName := "user_" + strings.ReplaceAll(req.UserId, "-", "_") <-- example
// why? so we can load the partition in before the comparison query even hits this service

// func(context, text chunk messages, embeddings corresponding to the text chunks)
//   - inserts the given text chunks into the database with their corresponding embeddings
//   - assumes: the user id field in the text chunk message is true and accurate
func (s *vectorServer) insertRows(ctx context.Context, textChunkMessages []*pb.TextChunkMessage, embeddings [][]byte) error {
	var dateCreatedValues []int64
	var dateModifiedValues []int64
	var filePaths []string
	var userIDs []string
	var resourceTypes []string
	var starts []int64
	var ends []int64
	var chunkIds []string
	var titles []string
	var platforms []int8
	var services []string
	var fileIds []string
	var fileUrls []string
	dimension, err := strconv.Atoi(os.Getenv("VECTOR_DIMENSION"))
	if err != nil {
		return err
	}

	for _, textChunkMessage := range textChunkMessages {
		if textChunkMessage.Metadata.DateCreated == nil {
			textChunkMessage.Metadata.DateCreated = timestamppb.New(time.Unix(0, 0))
		}
		if textChunkMessage.Metadata.DateLastModified == nil {
			textChunkMessage.Metadata.DateLastModified = timestamppb.New(time.Unix(0, 0))
		}

		dateCreatedValues = append(dateCreatedValues, textChunkMessage.Metadata.DateCreated.Seconds)
		dateModifiedValues = append(dateModifiedValues, textChunkMessage.Metadata.DateLastModified.Seconds)
		filePaths = append(filePaths, textChunkMessage.Metadata.FilePath)
		userIDs = append(userIDs, textChunkMessage.Metadata.UserId)
		starts = append(starts, int64(textChunkMessage.Metadata.Start))
		ends = append(ends, int64(textChunkMessage.Metadata.End))
		chunkIds = append(chunkIds, textChunkMessage.Metadata.ChunkId)
		resourceTypes = append(resourceTypes, textChunkMessage.Metadata.ResourceType)
		titles = append(titles, textChunkMessage.Metadata.Title)
		platforms = append(platforms, int8(textChunkMessage.Metadata.Platform))
		services = append(services, textChunkMessage.Metadata.Service)
		fileIds = append(fileIds, textChunkMessage.Metadata.FileId)
		fileUrls = append(fileUrls, textChunkMessage.Metadata.FileUrl)
	}

	_, err = s.milvusClient.Insert(ctx,
		s.collectionName,
		"",
		entity.NewColumnInt64("date_created", dateCreatedValues),
		entity.NewColumnInt64("date_modified", dateModifiedValues),
		entity.NewColumnVarChar("resource_type", resourceTypes),
		entity.NewColumnVarChar("file_path", filePaths),
		entity.NewColumnVarChar("user_id", userIDs),
		entity.NewColumnInt64("start", starts),
		entity.NewColumnInt64("end", ends),
		entity.NewColumnVarChar("title", titles),
		entity.NewColumnVarChar("chunk_id", chunkIds),
		entity.NewColumnInt8("platform", platforms),
		entity.NewColumnVarChar("file_id", fileIds),
		entity.NewColumnVarChar("service", services),
		entity.NewColumnVarChar("file_url", fileUrls),
		entity.NewColumnBinaryVector("vector", dimension, embeddings),
	)
	if err != nil {
		return err
	}
	return nil
}

// func(context)
//   - sets up a collection in the database with hardcoded fields as defined in this function
//   - assumes: milvus client is connected
func (s *vectorServer) setupCollection(ctx context.Context, collectionName string) error {
	// Check if collection already exists
	exists, err := s.milvusClient.HasCollection(ctx, collectionName)
	if err != nil {
		return fmt.Errorf("failed to check if collection exists: %w", err)
	}

	// If collection already exists, load it and return
	if exists {
		if err = s.milvusClient.LoadCollection(ctx, collectionName, false); err != nil {
			return fmt.Errorf("failed to load the collection into memory: %w", err)
		}
		return nil
	}

	dimension, err := strconv.Atoi(os.Getenv("VECTOR_DIMENSION"))
	if err != nil {
		return fmt.Errorf("failed to parse .env vector dimension value: %w", err)
	}

	// define the schema
	idField := entity.NewField().WithName("id").WithDataType(entity.FieldTypeInt64).WithIsPrimaryKey(true).WithIsAutoID(true)
	dateCreatedField := entity.NewField().WithName("date_created").WithDataType(entity.FieldTypeInt64)
	dateLastModifiedField := entity.NewField().WithName("date_modified").WithDataType(entity.FieldTypeInt64)
	resourceTypeField := entity.NewField().WithName("resource_type").WithDataType(entity.FieldTypeVarChar).WithTypeParams(entity.TypeParamMaxLength, "255")
	userIdField := entity.NewField().WithName("user_id").WithDataType(entity.FieldTypeVarChar).WithTypeParams(entity.TypeParamMaxLength, "255")
	filePathField := entity.NewField().WithName("file_path").WithDataType(entity.FieldTypeVarChar).WithTypeParams(entity.TypeParamMaxLength, "255")
	chunkStartField := entity.NewField().WithName("start").WithDataType(entity.FieldTypeInt64)
	chunkEndField := entity.NewField().WithName("end").WithDataType(entity.FieldTypeInt64)
	chunkIdField := entity.NewField().WithName("chunk_id").WithDataType(entity.FieldTypeVarChar).WithTypeParams(entity.TypeParamMaxLength, "255")
	titleField := entity.NewField().WithName("title").WithDataType(entity.FieldTypeVarChar).WithTypeParams(entity.TypeParamMaxLength, "255")
	serviceField := entity.NewField().WithName("service").WithDataType(entity.FieldTypeVarChar).WithTypeParams(entity.TypeParamMaxLength, "255")
	platformField := entity.NewField().WithName("platform").WithDataType(entity.FieldTypeInt8)
	fileIdField := entity.NewField().WithName("file_id").WithDataType(entity.FieldTypeVarChar).WithTypeParams(entity.TypeParamMaxLength, "255")
	fileUrlField := entity.NewField().WithName("file_url").WithDataType(entity.FieldTypeVarChar).WithTypeParams(entity.TypeParamMaxLength, "255")
	// create a binary vector field
	vector := entity.NewField().WithName("vector").WithDataType(entity.FieldTypeBinaryVector).WithDim(int64(dimension))

	schema := entity.NewSchema().WithName(collectionName).
		WithField(idField).
		WithField(dateCreatedField).
		WithField(dateLastModifiedField).
		WithField(userIdField).
		WithField(filePathField).
		WithField(resourceTypeField).
		WithField(chunkStartField).
		WithField(chunkEndField).
		WithField(chunkIdField).
		WithField(titleField).
		WithField(platformField).
		WithField(fileIdField).
		WithField(serviceField).
		WithField(fileUrlField).
		WithField(vector)

	// create the collection
	err = s.milvusClient.CreateCollection(ctx, schema, 1, client.WithConsistencyLevel(entity.ClBounded))
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}

	// create a vector index
	index, err := entity.NewIndexBinIvfFlat(entity.HAMMING, 256) // nlist {128-1024} larger is faster search, slower build
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}
	err = s.milvusClient.CreateIndex(ctx, collectionName, "vector", index, false, client.WithIndexName("vector_index"))
	if err != nil {
		return fmt.Errorf("failed to set up index in database: %w", err)
	}

	// load the collection
	if err = s.milvusClient.LoadCollection(ctx, collectionName, false); err != nil {
		return fmt.Errorf("failed to load the collection into memory: %w", err)
	}
	return nil
}
