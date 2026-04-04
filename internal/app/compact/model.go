// Package compact provides message types for conversation compaction.
package compact

// CompactResultMsg is sent when a compaction operation completes.
type CompactResultMsg struct {
	Summary       string
	OriginalCount int
	Trigger       string // "manual" or "auto"
	Error         error
}

// TokenLimitResultMsg is sent when a token limit fetch completes.
type TokenLimitResultMsg struct {
	Result string
	Error  error
}
