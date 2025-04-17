package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	pb "github.com/cc-0000/indeq/common/api"
	"github.com/google/generative-ai-go/genai"
	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/api/iterator"
)

var allowedModels = map[string]string{
	"gemini-2.0-flash":             "gemini-2.0-flash",
	"llama-4.0-maverick":           "meta-llama/Llama-4-Maverick-17B-128E-Instruct-FP8",
	"qwq-32b":                      "Qwen/QwQ-32B",
	"gpt-4o-mini":                  "gpt-4o-mini",
	"deepseek-r1-distill-qwen-32b": "deepseek-ai/DeepSeek-R1-Distill-Qwen-32B",
	// "claude-3-7-sonnet":         "anthropic/claude-3-7-sonnet-latest", // TODO: enable this for PRO users only
}

var deepInfraModels = map[string]struct{}{
	"llama-4.0-maverick":           {},
	"qwq-32b":                      {},
	"deepseek-r1-distill-qwen-32b": {},
	// "claude-3-7-sonnet":            {}, // TODO: enable this for PRO users only
}

var openAiModels = map[string]struct{}{
	"gpt-4o-mini": {},
}

var thinkingModels = map[string]struct{}{
	"qwq-32b":                      {},
	"deepseek-r1-distill-qwen-32b": {},
}

func modelAllowed(model string) bool {
	_, ok := allowedModels[model]
	return ok
}

func (s *queryServer) sendToOpenApiModel(ctx context.Context, model string, conversation *pb.QueryConversation, fullprompt string, llmResponse *string, reasoningResponse *string, queue amqp.Queue, channel *amqp.Channel) error {
	// Prepare the request URL
	apiURL := "https://api.deepinfra.com/v1/openai/chat/completions"
	if _, ok := openAiModels[model]; ok {
		apiURL = "https://api.openai.com/v1/chat/completions"
	}

	// Convert conversation history to messages format for the API
	messages := []map[string]string{
		{
			"role":    "system",
			"content": s.systemPrompt,
		},
	}

	// Add conversation history
	for _, message := range conversation.SummarizedMessages {
		var role string
		if message.Sender == "model" {
			role = "assistant"
		} else if message.Sender == "user" {
			role = "user"
		}
		messages = append(messages, map[string]string{
			"role":    role,
			"content": message.Text,
		})
	}

	// Add the current query as the last user message
	messages = append(messages, map[string]string{
		"role":    "user",
		"content": fullprompt,
	})

	// Prepare request body
	requestBody := map[string]interface{}{
		"model":    allowedModels[model],
		"stream":   true,
		"messages": messages,
	}

	// Marshal the request body to JSON
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Create the HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add headers
	req.Header.Set("Content-Type", "application/json")
	if _, ok := openAiModels[model]; ok {
		req.Header.Set("Authorization", "Bearer "+s.openAiApiKey)
	} else if _, ok := deepInfraModels[model]; ok {
		req.Header.Set("Authorization", "Bearer "+s.deepInfraApiKey)
	}

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request to DeepInfra API: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OpenAI/DeepInfra API returned non-200 status code: %d, body: %s", resp.StatusCode, string(body))
	}

	// if this is a thinking model, we need to send the thinking token'
	reasoning := false
	if _, ok := thinkingModels[model]; ok {
		//</think>
		// Check if the queue still exists
		_, err := channel.QueueDeclarePassive(
			queue.Name, // queue name
			false,      // durable
			true,       // delete when unused
			false,      // exclusive
			false,      // no-wait
			amqp.Table{ // arguments
				"x-expires": s.queueTTL, // TTL in milliseconds
			},
		)

		if err == nil {
			// Queue exists, send the token
			queueTokenMessage := &pb.QueryTokenMessage{
				Type: "think_start",
			}

			byteMessage, err := json.Marshal(queueTokenMessage)
			if err != nil {
				log.Printf("Error marshalling token message: %v", err)
			}

			err = s.sendToQueue(ctx, channel, queue.Name, byteMessage)
			if err != nil {
				log.Printf("Error publishing message: %v", err)
			}
		}
		reasoning = true
	}

	// Process the SSE stream
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines and the [DONE] message
		if line == "" || strings.HasPrefix(line, "ping:") || line == "data: [DONE]" {
			continue
		}

		// Extract the data part
		data := strings.TrimPrefix(line, "data: ")

		// Parse the JSON response
		var response struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}

		if err := json.Unmarshal([]byte(data), &response); err != nil {
			log.Printf("Error parsing SSE data: %v", err)
			continue
		}

		// Process each choice
		for _, choice := range response.Choices {
			// Skip if there's no content or it's the special end token
			if choice.Delta.Content == "" || choice.Delta.Content == "</s>" {
				continue
			}

			// Check if the queue still exists
			_, err := channel.QueueDeclarePassive(
				queue.Name, // queue name
				false,      // durable
				true,       // delete when unused
				false,      // exclusive
				false,      // no-wait
				amqp.Table{ // arguments
					"x-expires": s.queueTTL, // TTL in milliseconds
				},
			)

			queueTokenMessage := &pb.QueryTokenMessage{
				Type:  "token",
				Token: choice.Delta.Content,
			}
			if choice.Delta.Content == "</think>" { // detect think end
				queueTokenMessage.Type = "think_end"
				queueTokenMessage.Token = "THINKING HAS ENDED"
				reasoning = false
			}

			if err == nil {
				// Queue exists, send the token
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

			// Append to the complete response
			if choice.Delta.Content != "</think>" {
				if reasoning {
					*reasoningResponse += choice.Delta.Content
				} else {
					*llmResponse += choice.Delta.Content
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading response stream: %w", err)
	}

	return nil
}

func (s *queryServer) sendToGemini(ctx context.Context, conversation *pb.QueryConversation, fullprompt string, llmResponse *string, queue amqp.Queue, channel *amqp.Channel) error {
	session := s.geminiFlash2ModelHeavy.StartChat()
	session.History = s.convertConversationToSummarizedChatHistory(conversation)

	// send the message to the llm
	iter := session.SendMessageStream(ctx, genai.Text(fullprompt))

	// send the tokens
	for {
		resp, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("error streaming response from gemini: %w", err)
		}

		for _, candidate := range resp.Candidates {
			for _, part := range candidate.Content.Parts {
				// check if the queue still exists
				_, err := channel.QueueDeclarePassive(
					queue.Name, // queue name
					false,      // durable
					true,       // delete when unused
					false,      // exclusive
					false,      // no-wait
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
				*llmResponse += fmt.Sprintf("%v", part)
			}
		}
	}

	return nil
}
