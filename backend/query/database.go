package main

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/protobuf/encoding/protojson"

	pb "github.com/cc-0000/indeq/common/api"
	"github.com/google/generative-ai-go/genai"
)

/***************************
** OWNERSHIP MAPPING CRUD **
***************************/

// func(context, user id)
//   - creates an empty ownership mapping for the given user
func (s *queryServer) createEmptyOwnershipMapping(ctx context.Context, userId string) error {
	conversationHeaders := []*pb.QueryConversationHeader{}
	headerBytes, err := protojson.Marshal(&pb.QueryOwnershipMapping{ConversationHeaders: conversationHeaders})
	if err != nil {
		return fmt.Errorf("failed to marshal empty conversation headers: %w", err)
	}

	doc := map[string]any{
		"conversation_headers": string(headerBytes),
	}
	_, err = s.ownershipDB.Put(ctx, userId, doc)
	if err != nil {
		return fmt.Errorf("failed to create user ownership mapping: %w", err)
	}

	return nil
}

// func(context, user id)
//   - returns an ownership mapping for the given user
//   - if the ownership mapping doesn't exist (aka the user doesn't have conversations), it will create one
func (s *queryServer) getOwnershipMapping(ctx context.Context, userId string) ([]*pb.QueryConversationHeader, error) {
	// first check if the user exists
	exists := true
	row := s.ownershipDB.Get(ctx, userId)
	if row.Err() != nil {
		if row.Err().Error() == "Not Found: missing" {
			exists = false
		} else {
			return nil, fmt.Errorf("failed to check if user exists: %w", row.Err())
		}
	}

	// this means the user doesn't have any conversations
	if !exists {
		if err := s.createEmptyOwnershipMapping(ctx, userId); err != nil {
			return nil, fmt.Errorf("failed to create new user ownership mapping: %w", err)
		}
		return []*pb.QueryConversationHeader{}, nil
	}

	// this means the user already has some x number of conversations in a mapping --> fetch that
	var doc map[string]any
	if err := row.ScanDoc(&doc); err != nil {
		return nil, fmt.Errorf("failed to scan conversation headers: %w", err)
	}
	headerJSON, ok := doc["conversation_headers"].(string)
	if !ok {
		return nil, fmt.Errorf("conversation_headers is not a string: %T", doc["conversation_headers"])
	}

	var headerList pb.QueryOwnershipMapping
	if err := protojson.Unmarshal([]byte(headerJSON), &headerList); err != nil {
		return nil, fmt.Errorf("failed to unmarshal conversation headers: %w", err)
	}

	return headerList.ConversationHeaders, nil
}

// func(context, user id, conversation headers)
//   - updates the ownership mapping for the given user with the given conversation headers
//   - assumes: the user already has a ownership mapping
func (s *queryServer) updateOwnershipMapping(ctx context.Context, userId string, conversationHeaders []*pb.QueryConversationHeader) error {
	row := s.ownershipDB.Get(ctx, userId)
	if row.Err() != nil {
		return fmt.Errorf("failed to get user ownership mapping: %w", row.Err())
	}

	// get the existing document
	var doc map[string]any
	if err := row.ScanDoc(&doc); err != nil {
		return fmt.Errorf("failed to scan user ownership mapping: %w", err)
	}

	// Marshal the conversation headers into a JSON string
	headerBytes, err := protojson.Marshal(&pb.QueryOwnershipMapping{ConversationHeaders: conversationHeaders})
	if err != nil {
		return fmt.Errorf("failed to marshal conversation headers: %w", err)
	}

	// Update the document in CouchDB
	doc["conversation_headers"] = string(headerBytes)
	_, err = s.ownershipDB.Put(ctx, userId, doc)
	if err != nil {
		return fmt.Errorf("failed to update user ownership mapping: %w", err)
	}

	return nil
}

/**********************
** CONVERSATION CRUD **
***********************/

// func(context, conversation id, title)
//   - creates a new empty conversation in the database
func (s *queryServer) createEmptyConversation(ctx context.Context, conversationId string, title string) error {
	// Create a new empty conversation
	conversation := &pb.QueryConversation{
		ConversationId:     conversationId,
		Title:              title,
		SummarizedMessages: []*pb.QueryMessage{},
		FullMessages:       []*pb.QueryMessage{},
	}

	// store the conversation in the database
	conversationJSON, err := protojson.Marshal(conversation)
	if err != nil {
		return fmt.Errorf("failed to marshal conversation: %w", err)
	}

	doc := map[string]any{
		"conversation": string(conversationJSON),
	}
	_, err = s.conversationsDB.Put(ctx, conversationId, doc)
	if err != nil {
		return fmt.Errorf("failed to create new conversation: %w", err)
	}

	return nil
}

// func(context, conversation id)
//   - returns the conversation with the given conversation id, or an error if it doesn't exist
func (s *queryServer) getConversation(ctx context.Context, conversationId string) (*pb.QueryConversation, error) {
	// first check if the conversation exists
	row := s.conversationsDB.Get(ctx, conversationId)
	if row.Err() != nil {
		return nil, fmt.Errorf("failed to get conversation: %w", row.Err())
	}

	var doc map[string]any
	if err := row.ScanDoc(&doc); err != nil {
		return nil, fmt.Errorf("failed to scan conversation: %w", err)
	}

	conversationJSON := doc["conversation"]
	conversation := &pb.QueryConversation{}
	if err := protojson.Unmarshal([]byte(conversationJSON.(string)), conversation); err != nil {
		return nil, fmt.Errorf("failed to unmarshal conversation: %w", err)
	}

	return conversation, nil
}

// func(context, conversation id, conversation)
//   - updates the conversation with the given conversation id with the given conversation
//   - assumes: the conversation exists in the database
func (s *queryServer) updateConversation(ctx context.Context, conversationId string, conversation *pb.QueryConversation) error {
	row := s.conversationsDB.Get(ctx, conversationId)
	if row.Err() != nil {
		return fmt.Errorf("failed to get conversation: %w", row.Err())
	}

	// get the existing document
	var doc map[string]any
	if err := row.ScanDoc(&doc); err != nil {
		return fmt.Errorf("failed to scan conversation: %w", err)
	}
	conversationJSON, err := protojson.Marshal(conversation)
	if err != nil {
		return fmt.Errorf("failed to marshal conversation: %w", err)
	}

	// Update the document in CouchDB
	doc["conversation"] = string(conversationJSON)
	_, err = s.conversationsDB.Put(ctx, conversationId, doc)
	if err != nil {
		return fmt.Errorf("failed to update conversation: %w", err)
	}

	return nil
}

// func(context, conversation id)
//   - deletes the conversation with the given conversation id from the database
//   - assumes: the conversation exists in the database
func (s *queryServer) deleteConversation(ctx context.Context, conversationId string) error {
	row := s.conversationsDB.Get(ctx, conversationId)
	if row.Err() != nil {
		return fmt.Errorf("failed to get conversation document for deletion: %w", row.Err())
	}

	var doc map[string]any
	if err := row.ScanDoc(&doc); err != nil {
		return fmt.Errorf("failed to scan conversation document for deletion: %w", err)
	}

	// Delete the document
	_, err := s.conversationsDB.Delete(ctx, conversationId, doc["_rev"].(string))
	if err != nil {
		return fmt.Errorf("failed to delete conversation from database: %w", err)
	}

	return nil
}

// func(context, conversation id)
//   - checks if a conversation exists in the database
//   - returns true if the conversation exists, false otherwise
//   - empty conversations ("") can never exist
//   - assumes: database is connected
func (s *queryServer) conversationExists(ctx context.Context, conversationID string) (bool, error) {
	if len(conversationID) == 0 {
		return false, nil
	}
	row := s.conversationsDB.Get(ctx, conversationID)
	if row.Err() != nil {
		// If the error indicates that the document doesn't exist, return false
		if row.Err().Error() == "Not Found: missing" {
			return false, nil
		}
		// For any other error, return it
		return false, fmt.Errorf("failed to check if conversation exists: %w", row.Err())
	}
	return true, nil
}

/*********************
** HELPER FUNCTIONS **
**********************/

// func(conversation)
//   - takes a conversation and converts the SummarizedMessages to []*genai.Content
func (s *queryServer) convertConversationToSummarizedChatHistory(conversation *pb.QueryConversation) []*genai.Content {
	var chatHistory []*genai.Content
	for _, message := range conversation.SummarizedMessages {
		if message.Sender == "user" {
			chatHistory = append(chatHistory, &genai.Content{
				Parts: []genai.Part{
					genai.Text(message.Text),
				},
				Role: "user",
			})
		} else if message.Sender == "model" {
			chatHistory = append(chatHistory, &genai.Content{
				Parts: []genai.Part{
					genai.Text(message.Text),
				},
				Role: "model",
			})
		}
	}
	return chatHistory
}

// func(context, conversation)
//   - summarizes the SummarizedMessages of the conversation, if needed and returns the updated object
func (s *queryServer) summarizeConversation(ctx context.Context, conversation *pb.QueryConversation) (*pb.QueryConversation, error) {
	// parse out the summarized chat history []*genai.Content
	chatHistory := s.convertConversationToSummarizedChatHistory(conversation)
	tokenCount := 0
	if len(chatHistory) > 0 {
		chatHistoryParts := make([]genai.Part, len(chatHistory))
		for i, content := range chatHistory {
			chatHistoryParts[i] = content.Parts[0].(genai.Text)
		}
		tokenCountResponse, err := s.geminiFlash2ModelSummarization.CountTokens(ctx, chatHistoryParts...)
		if err != nil {
			return nil, fmt.Errorf("failed to count tokens: %w", err)
		}
		tokenCount = int(tokenCountResponse.TotalTokens)
	}

	log.Print(tokenCount)
	if tokenCount > s.summaryUpperBound {
		// Summarize the older messages first to keep the most recent conversation intact
		totalTokens := 0
		endIdx := 1

		// Find the cutoff point working backward from the end
		for i := len(chatHistory) - 1; i >= 0; i-- {
			tokenCount, err := s.geminiFlash2ModelSummarization.CountTokens(ctx, chatHistory[i].Parts[0].(genai.Text))
			if err != nil {
				return nil, fmt.Errorf("failed to count tokens: %w", err)
			}
			totalTokens += int(tokenCount.TotalTokens)
			// if we've exceeded the summary lower bound, this means anything including and before this point should be summarized
			if totalTokens > s.summaryLowerBound {
				endIdx = i + 1
				break
			}
		}

		session := s.geminiFlash2ModelSummarization.StartChat()
		session.History = chatHistory[:endIdx] // set the chat history to the messages that need to be summarized

		command := "Instruction: Summarize the above conversation between a human and an AI assistant."
		summaryResponse, err := session.SendMessage(ctx, genai.Text(command))
		if err != nil {
			return nil, fmt.Errorf("failed to send message to google gemini: %w", err)
		}

		var summaryText string
		if len(summaryResponse.Candidates) > 0 && len(summaryResponse.Candidates[0].Content.Parts) > 0 {
			if textPart, ok := summaryResponse.Candidates[0].Content.Parts[0].(genai.Text); ok {
				summaryText = string(textPart)
			}
		}

		conversation.SummarizedMessages = append([]*pb.QueryMessage{
			{
				Sender: "model",
				Text:   summaryText,
			},
		}, conversation.SummarizedMessages[endIdx:]...)
	}

	return conversation, nil
}
