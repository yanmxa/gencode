// Package moonshot implements the LLMProvider interface using the Moonshot AI platform.
package moonshot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/provider"
)

// Client implements the LLMProvider interface for Moonshot AI.
type Client struct {
	apiKey string
	name   string
	endpoint string
}

// NewClient creates a new Moonshot client.
func NewClient(apiKey string, name string) *Client {
	return &Client{
		apiKey: apiKey,
		name:   name,
		endpoint: "https://api.moonshot.cn/v1", // Placeholder URL
	}
}

// Name returns the provider name.
func (c *Client) Name() string {
	return c.name
}

// Stream sends a completion request and returns a channel of streaming chunks.
func (c *Client) Stream(ctx context.Context, opts provider.CompletionOptions) <-chan provider.StreamChunk {
	ch := make(chan provider.StreamChunk)

	go func() {
		defer close(ch)

		// Prepare the request payload.
		payload, err := preparePayload(opts)
		if err != nil {
			log.LogError(c.name, err)
			ch <- provider.StreamChunk{
				Type:  provider.ChunkTypeError,
				Error: err,
			}
			return
		}

		// Create the HTTP request.
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint + "/chat/completions", payload)
		if err != nil {
			log.LogError(c.name, err)
			ch <- provider.StreamChunk{
				Type:  provider.ChunkTypeError,
				Error: err,
			}
			return
		}

		// Set headers.
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-KEY", c.apiKey)

		// Log request
		log.LogRequest(c.name, opts.Model, opts)

		// Make the HTTP request.
		client := &http.Client{}
		res, err := client.Do(req)
		if err != nil {
			log.LogError(c.name, err)
			ch <- provider.StreamChunk{
				Type:  provider.ChunkTypeError,
				Error: err,
			}
			return
		}
		defer res.Body.Close()

		// Check the response status code.
		if res.StatusCode != http.StatusOK {
			bodyBytes, err := io.ReadAll(res.Body)
			if err != nil {
				log.LogError(c.name, fmt.Errorf("HTTP error: %s (failed to read response body)", res.Status))
				ch <- provider.StreamChunk{
					Type:  provider.ChunkTypeError,
					Error: fmt.Errorf("HTTP error: %s (failed to read response body)", res.Status),
				}
				return
			}
			bodyString := string(bodyBytes)
			err = fmt.Errorf("HTTP error: %s, Body: %s", res.Status, bodyString)
			log.LogError(c.name, err)
			ch <- provider.StreamChunk{
				Type:  provider.ChunkTypeError,
				Error: err,
			}
			return
		}

		// Process the response stream.
		decoder := json.NewDecoder(res.Body)

		// Stream timing and counting
		streamStart := time.Now()
		chunkCount := 0

		var response provider.CompletionResponse

		for {
			var chunk map[string]interface{}
			err := decoder.Decode(&chunk)
			if err != nil {
				if err == io.EOF {
					break // End of stream
				}
				log.LogError(c.name, err)
				ch <- provider.StreamChunk{
					Type:  provider.ChunkTypeError,
					Error: err,
				}
				return
			}

			chunkCount++

			// Extract data from the chunk and send it to the channel.
			text, ok := chunk["choices"].([]interface{})
			if ok && len(text) > 0 {
				content, ok := text[0].(map[string]interface{})["delta"].(map[string]interface{})["content"].(string)
				if ok {
					ch <- provider.StreamChunk{
						Type: provider.ChunkTypeText,
						Text: content,
					}
					response.Content += content
				}
			}
		}

		// Log stream done
		log.LogStreamDone(c.name, time.Since(streamStart), chunkCount)

		// Log response
		log.LogResponse(c.name, response)

		ch <- provider.StreamChunk{
			Type:     provider.ChunkTypeDone,
			Response: &response,
		}
	}()

	return ch
}

// preparePayload prepares the JSON payload for the Moonshot API request.
func preparePayload(opts provider.CompletionOptions) (io.Reader, error) {
	// Convert messages to the expected format.
	messages := make([]map[string]interface{}, 0, len(opts.Messages))
	for _, msg := range opts.Messages {
		message := map[string]interface{}{}
		switch msg.Role {
		case "user":
			message["role"] = "user"
			message["content"] = msg.Content
		case "assistant":
			message["role"] = "assistant"
			message["content"] = msg.Content
		case "system":
			message["role"] = "system"
			message["content"] = msg.Content
		}
		messages = append(messages, message)
	}

	// Build the payload.
	payload := map[string]interface{}{}
	payload["model"] = opts.Model
	payload["messages"] = messages
	payload["stream"] = true // Enable streaming

	// Convert payload to JSON.
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return io.Reader(strings.NewReader(string(jsonPayload))), nil
}

// ListModels returns the available models for Moonshot AI.
func (c *Client) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	// In the absence of an API endpoint, return a hardcoded list of models.
	models := []provider.ModelInfo{
		{ID: "moonshot-standard", Name: "Moonshot Standard", DisplayName: "Moonshot Standard"},
		{ID: "moonshot-pro", Name: "Moonshot Pro", DisplayName: "Moonshot Pro"},
	}
	return models, nil
}

// NewAPIKeyClient creates a new Moonshot client using API Key authentication
func NewAPIKeyClient(ctx context.Context) (provider.LLMProvider, error) {
	apiKey := os.Getenv("MOONSHOT_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("MOONSHOT_API_KEY environment variable is not set")
	}

	return NewClient(apiKey, "moonshot:api_key"), nil
}

// Ensure Client implements LLMProvider
var _ provider.LLMProvider = (*Client)(nil)
