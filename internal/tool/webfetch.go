package tool

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

const (
	maxResponseSize = 5 * 1024 * 1024 // 5MB
	httpTimeout     = 30 * time.Second
)

// WebFetchTool fetches content from URLs
type WebFetchTool struct{}

func (t *WebFetchTool) Name() string        { return "WebFetch" }
func (t *WebFetchTool) Description() string { return "Fetch content from a URL" }
func (t *WebFetchTool) Icon() string        { return ui.IconWeb }

func (t *WebFetchTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	start := time.Now()

	urlStr, err := requireString(params, "url")
	if err != nil {
		return ui.NewErrorResult(t.Name(), err.Error())
	}

	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		urlStr = "https://" + urlStr
	}

	format := "markdown"
	if f := getString(params, "format"); f != "" {
		format = f
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: httpTimeout,
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return ui.NewErrorResult(t.Name(), "invalid URL: "+err.Error())
	}

	// Set user agent
	req.Header.Set("User-Agent", "GenCode/1.0")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return ui.NewErrorResult(t.Name(), "request failed: "+err.Error())
	}
	defer func() { _ = resp.Body.Close() }()

	// Check status code
	if resp.StatusCode >= 400 {
		return ui.NewErrorResult(t.Name(), fmt.Sprintf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode)))
	}

	// Read body with size limit
	limitedReader := io.LimitReader(resp.Body, maxResponseSize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return ui.NewErrorResult(t.Name(), "failed to read response: "+err.Error())
	}

	// Convert content based on format
	content := string(body)
	contentType := resp.Header.Get("Content-Type")

	if format == "markdown" && strings.Contains(contentType, "text/html") {
		// Convert HTML to Markdown
		converter := md.NewConverter("", true, nil)
		markdown, err := converter.ConvertString(content)
		if err == nil {
			content = markdown
		}
	}

	// Truncate if too long
	truncated := false
	lines := strings.Split(content, "\n")
	if len(lines) > maxReadLines {
		lines = lines[:maxReadLines]
		content = strings.Join(lines, "\n")
		truncated = true
	}

	duration := time.Since(start)

	result := ui.ToolResult{
		Success: true,
		Output:  content,
		Metadata: ui.ResultMetadata{
			Title:      t.Name(),
			Icon:       t.Icon(),
			Subtitle:   urlStr,
			Size:       int64(len(body)),
			StatusCode: resp.StatusCode,
			LineCount:  len(lines),
			Duration:   duration,
			Truncated:  truncated,
		},
	}

	return result
}

func init() {
	Register(&WebFetchTool{})
}
