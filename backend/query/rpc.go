package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	pb "github.com/cc-0000/indeq/common/api"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/google/generative-ai-go/genai"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"

	_ "github.com/go-kivik/kivik/v4/couchdb"
)

// rpc(context, user id)
//   - returns all conversation headers (title and the id) for a given user
//   - assumes: database is connected
func (s *queryServer) GetAllConversations(ctx context.Context, req *pb.QueryGetAllConversationsRequest) (*pb.QueryGetAllConversationsResponse, error) {
	conversationHeaders, err := s.getOwnershipMapping(ctx, req.UserId)
	if err != nil {
		return nil, fmt.Errorf("failed to get all conversations: %w", err)
	}

	return &pb.QueryGetAllConversationsResponse{
		ConversationHeaders: conversationHeaders,
	}, nil
}

// rpc(context, new conversation request)
//   - creates a new conversation in the database and returns the newly generated uuid
//   - assumes: the uuid will be uniquely generated within 5 attempts
func (s *queryServer) StartNewConversation(ctx context.Context, req *pb.StartNewConversationRequest) (*pb.StartNewConversationResponse, error) {
	newConversationId := uuid.New().String()

	// check for up to 5 times for uuid collision
	i := 0
	for i < 5 {
		exists, err := s.conversationExists(ctx, newConversationId)
		if err != nil {
			return nil, fmt.Errorf("failed to check if conversation exists: %w", err)
		}
		if !exists {
			break
		}
		newConversationId = uuid.New().String()
		i++
	}

	if i == 5 {
		return nil, fmt.Errorf("failed to generate a unique conversation id after 5 attempts")
	}

	// query --> title truncated to 256 characters
	title := req.Query
	if len(title) > 256 {
		title = title[:253] + "..."
	}

	// create the empty conversation
	if err := s.createEmptyConversation(ctx, newConversationId, title); err != nil {
		return nil, fmt.Errorf("failed to create new conversation: %w", err)
	}

	// assign this conversation to this user
	conversationHeaders, err := s.getOwnershipMapping(ctx, req.UserId)
	if err != nil {
		return nil, fmt.Errorf("failed to user's existing conversations: %w", err)
	}
	conversationHeaders = append(conversationHeaders, &pb.QueryConversationHeader{
		ConversationId: newConversationId,
		Title:          title,
	})
	if err := s.updateOwnershipMapping(ctx, req.UserId, conversationHeaders); err != nil {
		return nil, fmt.Errorf("failed to update user's ownership mapping: %w", err)
	}

	return &pb.StartNewConversationResponse{
		ConversationId: newConversationId,
	}, nil
}

// rpc(context, get conversation request)
//   - returns the conversation for a given user and conversation id or NIL if the user doesn't have/own that conversation
//   - assumes: database is connected
func (s *queryServer) GetConversation(ctx context.Context, req *pb.QueryGetConversationRequest) (*pb.QueryGetConversationResponse, error) {
	conversationHeaders, err := s.getOwnershipMapping(ctx, req.UserId)
	if err != nil {
		return nil, fmt.Errorf("failed to get user's existing conversations: %w", err)
	}

	for _, header := range conversationHeaders {
		if header.ConversationId == req.ConversationId {
			// get the conversation
			conversation, err := s.getConversation(ctx, req.ConversationId)
			if err != nil {
				return nil, fmt.Errorf("failed to get conversation: %w", err)
			}

			return &pb.QueryGetConversationResponse{
				Conversation: conversation,
			}, nil
		}
	}

	// this means we couldn't find that conversation for that user
	return nil, fmt.Errorf("COULD_NOT_FIND_CONVERSATION")
}

// rpc(context, delete conversation request)
//   - deletes the conversation associated with the user, unless the user doesn't own that conversation
func (s *queryServer) DeleteConversation(ctx context.Context, req *pb.QueryDeleteConversationRequest) (*pb.QueryDeleteConversationResponse, error) {
	// first make sure the user owns that conversation
	conversationHeaders, err := s.getOwnershipMapping(ctx, req.UserId)
	if err != nil {
		return nil, fmt.Errorf("failed to get user's existing conversations: %w", err)
	}

	deleted := false
	newHeaders := []*pb.QueryConversationHeader{}
	for _, header := range conversationHeaders {
		if header.ConversationId == req.ConversationId {
			deleted = true
		} else {
			newHeaders = append(newHeaders, header)
		}
	}

	if deleted {
		if err := s.updateOwnershipMapping(ctx, req.UserId, newHeaders); err != nil {
			return nil, fmt.Errorf("failed to remove conversation from user's conversations: %w", err)
		}
		if err := s.deleteConversation(ctx, req.ConversationId); err != nil {
			return nil, fmt.Errorf("failed to delete conversation: %w", err)
		}
		return &pb.QueryDeleteConversationResponse{}, nil
	}

	return nil, fmt.Errorf("user does not own conversation %s", req.ConversationId)
}

// rpc(context, query request)
//   - takes in a query for a given user
//   - fetches the top k chunks relevant to the query and passes that context to the llm
//   - streams the response back to a rabbitMQ queue {conversation_id}
//   - assumes: you have set the variable s.queueTTL
func (s *queryServer) MakeQuery(ctx context.Context, req *pb.QueryRequest) (*pb.QueryResponse, error) {
	// get the number of sources and ttl from env
	kVal, err := strconv.ParseInt(os.Getenv("QUERY_NUMBER_OF_SOURCES"), 10, 32)
	if err != nil {
		return &pb.QueryResponse{}, fmt.Errorf("failed to retrieve the number_of_sources env variable: %w", err)
	}
	ttlVal, err := strconv.ParseUint(os.Getenv("QUERY_TTL"), 10, 32)
	if err != nil {
		return &pb.QueryResponse{}, fmt.Errorf("failed to retrieve the ttl env variable: %w", err)
	}

	// TODO: implement function calling for better filtering (dates, titles, etc.)
	// TODO: implement query conversion for better searching

	expandedQuery, err := s.expandQuery(ctx, req.Query, req.ConversationId)
	if err != nil {
		return &pb.QueryResponse{}, fmt.Errorf("failed to expand query: %w", err)
	}
	log.Print("got the expanded query: ", expandedQuery)

	// fetch context associated with the query
	topKChunksResponse, err := s.retrievalService.RetrieveTopKChunks(ctx, &pb.RetrieveTopKChunksRequest{
		UserId:         req.UserId,
		Prompt:         req.Query,
		ExpandedPrompt: expandedQuery,
		K:              int32(kVal),
		Ttl:            uint32(ttlVal),
	})
	if err != nil {
		topKChunksResponse = &pb.RetrieveTopKChunksResponse{
			TopKChunks: []*pb.TextChunkMessage{},
		}
	}

	// Group chunks by file path
	chunksByFilePath := make(map[string][]*pb.TextChunkMessage)
	for _, chunk := range topKChunksResponse.TopKChunks {
		filePath := chunk.Metadata.FilePath
		chunksByFilePath[filePath] = append(chunksByFilePath[filePath], chunk)
	}

	for filePath, chunks := range chunksByFilePath {
		sort.Slice(chunks, func(i, j int) bool {
			return chunks[i].Metadata.Start < chunks[j].Metadata.Start
		})
		chunksByFilePath[filePath] = chunks
	}

	// assemble the full prompt from the chunks and the query
	var fullprompt string = ""

	if len(chunksByFilePath) == 0 {
		fullprompt += "Question: " + req.Query + "\n\n"
		fullprompt += "Instructions: Provide a comprehensive answer to the question above, using the conversation history as context, as there are no excerpts to use."
	} else {
		excerptNumber := 1
		for _, chunks := range chunksByFilePath {
			fullprompt += fmt.Sprintf("Excerpt %d:\n", excerptNumber)
			fullprompt += fmt.Sprintf("Title: %s\n", chunks[0].Metadata.Title)
			fullprompt += fmt.Sprintf("Folder: %s\n", filepath.Dir(chunks[0].Metadata.FilePath))
			fullprompt += fmt.Sprintf("Date Modified: %s\n\n", chunks[0].Metadata.DateLastModified.AsTime().Format("2006-01-02 15:04:05"))

			for _, chunk := range chunks {
				content := chunk.Content
				fullprompt += fmt.Sprintf("Content: %s\n\n", content)
			}
			excerptNumber++
		}

		fullprompt += "Question: " + req.Query + "\n\n"
		fullprompt += "Instructions: Provide a comprehensive answer to the question above, using the given excerpts plus the conversation history if necessary, but falling back to your expert general knowledge if the excerpts are insufficient. Cite sources using the <Excerpt number> (with angle brackets!) of the document."
	}

	// TODO: add the option to use more than 1 model

	// start a gemini session and pull in the chat history
	conversation, err := s.getConversation(ctx, req.ConversationId)
	if err != nil {
		return &pb.QueryResponse{}, fmt.Errorf("failed to get conversation: %w", err)
	}
	session := s.geminiFlash2ModelHeavy.StartChat()
	session.History = s.convertConversationToSummarizedChatHistory(conversation)

	// send the message to the llm
	iter := session.SendMessageStream(ctx, genai.Text(fullprompt))
	var llmResponse string
	var sources []*pb.QuerySourceMessage

	// Create a rabbitmq channel to stream the response
	channel, err := s.rabbitMQConn.Channel()
	if err != nil {
		return &pb.QueryResponse{}, fmt.Errorf("failed to create rabbitmq channel: %w", err)
	}
	defer channel.Close()

	// create a rabbitmq queue to stream tokens to
	queue, err := channel.QueueDeclare(
		req.ConversationId, // queue name
		false,              // durable
		true,               // delete when unused
		false,              // exclusive
		false,              // no-wait
		amqp.Table{ // arguments
			"x-expires": s.queueTTL,
		},
	)
	if err != nil {
		return &pb.QueryResponse{}, fmt.Errorf("failed to create queue: %w", err)
	}

	// send the sources first
	excerptNumber := 1
	for _, chunks := range chunksByFilePath {
		// create a QueueSourceMessage for each file group
		if len(chunks) == 0 {
			continue
		}
		queueSourceMessage := &pb.QuerySourceMessage{
			Type:          "source",
			ExcerptNumber: int32(excerptNumber),
			Title:         chunks[0].Metadata.Title[:len(chunks[0].Metadata.Title)-len(filepath.Ext(chunks[0].Metadata.FilePath))],
			Extension:     strings.TrimPrefix(filepath.Ext(chunks[0].Metadata.FilePath), "."),
			FilePath:      chunks[0].Metadata.FilePath,
			FileUrl:       chunks[0].Metadata.FileUrl,
		}
		// TODO: implement the correct file extension for google sourced documents
		if chunks[0].Metadata.Platform == pb.Platform_PLATFORM_GOOGLE {
			queueSourceMessage.Extension = "Google"
		}
		byteMessage, err := json.Marshal(queueSourceMessage)
		if err != nil {
			return &pb.QueryResponse{}, fmt.Errorf("failed to marshal source message: %w", err)
		}

		if err = s.sendToQueue(ctx, channel, queue.Name, byteMessage); err != nil {
			return &pb.QueryResponse{}, fmt.Errorf("failed to publish message: %w", err)
		}
		sources = append(sources, queueSourceMessage)
		excerptNumber++
	}

	// send the tokens
	for {
		resp, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return &pb.QueryResponse{}, fmt.Errorf("error streaming response from gemini: %w", err)
		}

		for _, candidate := range resp.Candidates {
			for _, part := range candidate.Content.Parts {
				// check if the queue still exists
				_, err := channel.QueueDeclarePassive(
					req.ConversationId, // queue name
					false,              // durable
					true,               // delete when unused
					false,              // exclusive
					false,              // no-wait
					amqp.Table{ // arguments
						"x-expires": s.queueTTL, // 5 minutes in milliseconds
					},
				)
				if err == nil {
					// only send tokens if the queue still exists
					// create our token type
					queueTokenMessage := &pb.QueryTokenMessage{
						Type:  "token",
						Token: fmt.Sprintf("%v", part),
					}
					byteMessage, err := json.Marshal(queueTokenMessage)
					if err != nil {
						log.Printf("Error marshalling token message: %v", err)
						continue
					}

					err = s.sendToQueue(ctx, channel, queue.Name, byteMessage)
					if err != nil {
						log.Printf("Error publishing message: %v", err)
						continue
					}
				}
				llmResponse += fmt.Sprintf("%v", part)
			}
		}
	}

	oldConversation, err := s.getConversation(ctx, req.ConversationId)
	if err != nil {
		return &pb.QueryResponse{}, fmt.Errorf("failed to get the old conversation: %w", err)
	}

	// store the new user query
	userMessage := &pb.QueryMessage{
		Text:   req.Query,
		Sender: "user",
	}
	oldConversation.FullMessages = append(oldConversation.FullMessages, userMessage)
	oldConversation.SummarizedMessages = append(oldConversation.SummarizedMessages, userMessage)

	llmMessage := &pb.QueryMessage{
		Text:      llmResponse,
		Sender:    "model",
		Sources:   sources,
		Reasoning: []string{}, // TODO: implement reasoning for reasoning models
	}
	oldConversation.FullMessages = append(oldConversation.FullMessages, llmMessage)
	oldConversation.SummarizedMessages = append(oldConversation.SummarizedMessages, llmMessage)

	oldConversation, err = s.summarizeConversation(ctx, oldConversation)
	if err != nil {
		return &pb.QueryResponse{}, fmt.Errorf("failed to summarize conversation: %w", err)
	}

	err = s.updateConversation(ctx, req.ConversationId, oldConversation)
	if err != nil {
		return &pb.QueryResponse{}, fmt.Errorf("failed to update conversation: %w", err)
	}

	// send the end token
	endMessage := &pb.QueryTokenMessage{
		Type:  "end",
		Token: "",
	}
	byteMessage, err := json.Marshal(endMessage)
	if err != nil {
		return &pb.QueryResponse{}, fmt.Errorf("error marshalling token message: %w", err)
	}
	if err = s.sendToQueue(ctx, channel, queue.Name, byteMessage); err != nil {
		return &pb.QueryResponse{}, fmt.Errorf("error publishing message: %w", err)
	}

	return &pb.QueryResponse{}, nil
}

// func(context, rabbitmq channel, queue to send message to, byte encoded message of some json)
//   - sends the byte message to the given queue name
//   - assumes: the rabbitmq channel is open and the byte encoded message is from json
func (s *queryServer) sendToQueue(ctx context.Context, channel *amqp.Channel, queueName string, byteMessage []byte) error {
	err := channel.PublishWithContext(
		ctx,
		"",        // exchange
		queueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        byteMessage,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}
	return nil
}

// func(context, query to expand, conversation id)
//   - takes in a query and returns the expanded query that ideally contains better keywords for search
//   - can be set to return the original query if the env variable QUERY_EXPANSION is set to false
func (s *queryServer) expandQuery(ctx context.Context, query string, conversationID string) (string, error) {
	if os.Getenv("QUERY_EXPANSION") == "false" {
		return query, nil
	}

	fullprompt := "User Query: {" + query + "}\n\n" +
		"Search Terms:"

	// feed the old conversation to the model
	conversation, err := s.getConversation(ctx, conversationID)
	if err != nil {
		return query, fmt.Errorf("failed to get conversation: %w", err)
	}
	session := s.geminiFlash2ModelLight.StartChat()
	session.History = s.convertConversationToSummarizedChatHistory(conversation)

	// send the new message
	resp, err := session.SendMessage(ctx, genai.Text(fullprompt))
	if err != nil {
		return query, fmt.Errorf("failed to send message to google gemini: %w", err)
	}
	var messageText string
	if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
		if textPart, ok := resp.Candidates[0].Content.Parts[0].(genai.Text); ok {
			messageText = string(textPart)
		}
	}

	return messageText, nil
}
