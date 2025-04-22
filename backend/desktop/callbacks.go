package main

import (
	"context"
	"fmt"
	"log"
	"maps"
	"slices"
	"time"

	pb "github.com/cc-0000/indeq/common/api"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
)

// callback(mqtt client, incoming message)
//   - receive a set of files that the client wants to 'sync'
func (s *desktopServer) handleCrawlRequest(client mqtt.Client, msg mqtt.Message) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	userID := msg.Topic()[10:]
	log.Printf("Received new_crawl request from user: %s", userID)

	// make sure we're not already crawling
	wasCrawling, err := s.checkAndSetCrawling(ctx, userID)
	if err != nil {
		log.Printf("failed to check if user is crawling: %s", err)
		return
	}
	if wasCrawling {
		log.Print("user is already crawling. ignoring new request to crawl...")
		return
	}

	// parse out the new crawl
	newCrawl := &pb.NewCrawl{}
	if err := proto.Unmarshal([]byte(msg.Payload()), newCrawl); err != nil {
		log.Printf("failed to deserialize crawl request payload: %s", err)
		return
	}

	// Get the incoming paths and hashes
	newFilePaths := newCrawl.GetFilePaths()
	newFileHashes := newCrawl.GetFileHashes()
	hashToPath := make(map[string]string) // hash --> file_path
	pathToHash := make(map[string]string) // file_path --> hash
	for i := range newFilePaths {
		hashToPath[newFileHashes[i]] = newFilePaths[i]
		pathToHash[newFilePaths[i]] = newFileHashes[i]
	}
	fileRename := make(map[string]string) // file_path --> file_path
	toKeep := make(map[string]string)     // hash --> file_path
	var needToReqFiles []string
	var needToReqHashes []string

	// get all the old files from the database
	rows, err := s.getCurrentFiles(ctx, userID)
	if err != nil {
		log.Printf("failed to retrieve list of current indexed files from database: %s", err)
		return
	}
	defer rows.Close()

	// go through all the old files
	for rows.Next() {
		var oldFilePath, oldHash string
		var done bool
		if err := rows.Scan(&oldFilePath, &oldHash, &done); err != nil {
			log.Printf("failed to scan row while going through old files for user <%s> : %s", userID, err)
		}

		// check to see if the hash is still valid (aka we want to keep)
		if _, ok := hashToPath[oldHash]; ok && done {
			// check to see if this file needs to be renamed
			if oldFilePath != hashToPath[oldHash] {
				fileRename[oldFilePath] = hashToPath[oldHash]
			}
			toKeep[oldHash] = hashToPath[oldHash]
		}
	}

	// go through all the new files
	for _, newHash := range newFileHashes {
		if _, ok := toKeep[newHash]; !ok {
			// this file hash is not one of the one we kept so fetch new
			needToReqFiles = append(needToReqFiles, hashToPath[newHash])
			needToReqHashes = append(needToReqHashes, newHash)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		log.Printf("failed to begin crawl request transaction: %s", err)
		return
	}
	defer tx.Rollback()

	// delete unnecessary files
	toKeepHashes := slices.Collect(maps.Keys(toKeep))
	toKeepPaths := slices.Collect(maps.Values(toKeep))

	if err = batchDeleteFilepaths(ctx, tx, userID, toKeepHashes); err != nil {
		log.Printf("failed to delete files: %s", err)
		return
	}

	// also delete all vectors associated with the deleted files
	if _, err = s.vectorService.DeleteFiles(ctx, &pb.VectorFileDeleteRequest{
		UserId:    userID,
		Platform:  pb.Platform_PLATFORM_LOCAL,
		Files:     toKeepPaths,
		Exclusive: true,
	}); err != nil {
		log.Printf("failed to delete vectors associated with the files: %s", err)
		return
	}

	// update any file names that use the same hash
	if err = batchUpdateIndexedFiles(ctx, tx, userID, fileRename); err != nil {
		log.Printf("failed to update file names: %s", err)
		return
	}

	// set up new files to be uploaded
	if err = batchInsertFilepaths(ctx, tx, userID, needToReqFiles, needToReqHashes); err != nil {
		log.Printf("failed to create empty upload entries: %s", err)
		return
	}

	// update the file counts for crawl_stats
	if err = s.setStartingFileCount(ctx, tx, userID); err != nil {
		log.Printf("failed to set initial file counts for the crawl: %s", err)
		return
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		log.Printf("failed to commit transaction: %s", err)
		return
	}

	// request missing files from the desktop client
	if err = s.makeCrawlRequest(userID, needToReqFiles, needToReqHashes); err != nil {
		log.Print("failed to make crawl request back to the client ", err)
		return
	}
}

// func(user's ID, list of file paths that need to requested, list of file hashes that need to be requested)
//   - send a list of 'missing' files that need to be chunked and uploaded from the desktop client
func (s *desktopServer) makeCrawlRequest(userID string, needToReqFiles []string, needToReqHashes []string) error {
	crawlReq := &pb.NewCrawl{
		FilePaths:  needToReqFiles,
		FileHashes: needToReqHashes,
	}

	payload, err := proto.Marshal(crawlReq)
	if err != nil {
		return fmt.Errorf("failed to serialize the crawl request: %v", err)
	}

	s.mqttClient.Publish(fmt.Sprintf("crawl_req/%s", userID), 2, false, payload)

	return nil
}

// callback(mqtt client, incoming message)
//   - receive a chunk of a file (or <file_done> / <crawl_done> tags) from the client
func (s *desktopServer) handleChunk(client mqtt.Client, msg mqtt.Message) {
	ctx := context.Background()

	// user's can potentially lie about the chunks they own
	// resolve this by topic-based user id instead of believing what's in the chunk
	userID := msg.Topic()[10:]
	incomingChunk := &pb.TextChunkMessage{}
	proto.Unmarshal(msg.Payload(), incomingChunk)
	incomingChunk.Metadata.UserId = userID
	cleanedChunk, err := proto.Marshal(incomingChunk)
	if err != nil {
		log.Print("error serializing cleaned chunk: ", err)
		return
	}

	// Write the message to the kafka stream for down-stream processing
	message := kafka.Message{
		Value: cleanedChunk,
	}

	if err := s.kafkaWriter.WriteMessages(ctx, message); err != nil {
		log.Print("error writing mqtt message to kafka: ", err)
		return
	}
}

// callback(mqtt client, incoming message)
//   - receive a list of chunks that we supposedly requested
func (s *desktopServer) handleQueryResponse(client mqtt.Client, msg mqtt.Message) {
	// wait around 2 seconds before stopping the send (this could be due to channel being full or destroyed already)
	// this is to prevent locking the mutex for too long
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	queryResponse := &pb.DesktopChunkResponse{}
	if err := proto.Unmarshal([]byte(msg.Payload()), queryResponse); err != nil {
		log.Printf("failed to deserialize query response payload: %s", err)
		return
	}

	s.queryChannelMutex.Lock()
	defer s.queryChannelMutex.Unlock()
	ch, exists := s.queryChannels[queryResponse.RequestId]
	if exists {
		select {
		case <-ctx.Done():
			return
		case ch <- queryResponse:
			return
		}
	}
}
