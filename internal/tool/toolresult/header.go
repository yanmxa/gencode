package toolresult

import "time"

// ResultMetadata contains metadata about tool execution result
type ResultMetadata struct {
	Title      string        // Tool name
	Icon       string        // Tool icon
	Subtitle   string        // Short description (e.g., file path)
	Size       int64         // File/content size in bytes
	Duration   time.Duration // Execution duration
	LineCount  int           // Number of lines
	ItemCount  int           // Number of items (files/matches)
	StatusCode int           // HTTP status code (WebFetch)
	Truncated  bool          // Whether output was truncated
}
