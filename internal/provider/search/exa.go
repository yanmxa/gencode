package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	exaMCPEndpoint = "https://mcp.exa.ai/mcp"
)

// ExaProvider implements the Exa AI search provider
type ExaProvider struct{}

// NewExaProvider creates a new Exa provider
func NewExaProvider() *ExaProvider {
	return &ExaProvider{}
}

func (p *ExaProvider) Name() ProviderName    { return ProviderExa }
func (p *ExaProvider) DisplayName() string   { return "Exa AI" }
func (p *ExaProvider) RequiresAPIKey() bool  { return false }
func (p *ExaProvider) EnvVars() []string     { return []string{} }
func (p *ExaProvider) IsAvailable() bool     { return true } // Always available, no API key needed

// exaMCPRequest represents a JSON-RPC 2.0 request to Exa MCP
type exaMCPRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id"`
	Method  string         `json:"method"`
	Params  exaToolRequest `json:"params"`
}

type exaToolRequest struct {
	Name      string              `json:"name"`
	Arguments exaSearchArguments  `json:"arguments"`
}

type exaSearchArguments struct {
	Query            string `json:"query"`
	NumResults       int    `json:"numResults,omitempty"`
	Type             string `json:"type,omitempty"`
	Contents         exaContentsSpec `json:"contents"`
	IncludeDomains   []string `json:"includeDomains,omitempty"`
	ExcludeDomains   []string `json:"excludeDomains,omitempty"`
}

type exaContentsSpec struct {
	Text bool `json:"text"`
}

// exaMCPResponse represents a JSON-RPC 2.0 response from Exa MCP
type exaMCPResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *exaMCPError    `json:"error,omitempty"`
}

type exaMCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type exaToolResult struct {
	Content []exaToolContent `json:"content"`
}

type exaToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// exaSearchResult is the parsed search result from Exa
type exaSearchResult struct {
	Results []struct {
		Title string `json:"title"`
		URL   string `json:"url"`
		Text  string `json:"text"`
	} `json:"results"`
}

// Search performs a web search using Exa MCP endpoint
func (p *ExaProvider) Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
	numResults := opts.NumResults
	if numResults <= 0 {
		numResults = 8
	}

	// Build MCP request
	mcpReq := exaMCPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: exaToolRequest{
			Name: "web_search",
			Arguments: exaSearchArguments{
				Query:      query,
				NumResults: numResults,
				Type:       "auto",
				Contents: exaContentsSpec{
					Text: true,
				},
				IncludeDomains: opts.AllowedDomains,
				ExcludeDomains: opts.BlockedDomains,
			},
		},
	}

	body, err := json.Marshal(mcpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	client := &http.Client{Timeout: getTimeout(opts)}
	req, err := http.NewRequestWithContext(ctx, "POST", exaMCPEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse MCP response
	var mcpResp exaMCPResponse
	if err := json.Unmarshal(respBody, &mcpResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if mcpResp.Error != nil {
		return nil, fmt.Errorf("Exa error: %s", mcpResp.Error.Message)
	}

	// Parse the result
	var toolResult exaToolResult
	if err := json.Unmarshal(mcpResp.Result, &toolResult); err != nil {
		return nil, fmt.Errorf("failed to parse tool result: %w", err)
	}

	// The text content contains JSON with the search results
	if len(toolResult.Content) == 0 {
		return []SearchResult{}, nil
	}

	var searchData exaSearchResult
	for _, content := range toolResult.Content {
		if content.Type == "text" {
			if err := json.Unmarshal([]byte(content.Text), &searchData); err != nil {
				// Try to use the text directly if it's not JSON
				continue
			}
			break
		}
	}

	// Convert to SearchResult
	results := make([]SearchResult, 0, len(searchData.Results))
	for _, r := range searchData.Results {
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: truncateSnippet(r.Text, 200),
		})
	}

	return results, nil
}
