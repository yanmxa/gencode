package log

import "strings"

// requestLoggable is the interface that request options must satisfy
// for structured log output. Domain types satisfy this implicitly
// through duck typing -- no import of the log package is needed.
//
// Note: response logging uses llm.CompletionResponse via duck typing —
// the log package does not import core or llm. Request logging also uses duck
// typing because importing provider would create a circular dependency.
type requestLoggable interface {
	LogMaxTokens() int
	LogTemperature() float64
	LogSystemPrompt() string
	LogToolNames() []string
	LogMessageCount() int
	LogFormatMessages(sb *strings.Builder)
}

// requestDevData extends requestLoggable with raw data access for
// JSON dev directory output. The returned values are JSON-serializable
// domain types passed through as any to avoid domain imports.
type requestDevData interface {
	LogRawTools() any
	LogRawMessages() any
}
