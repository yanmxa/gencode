package log

import "strings"

// requestLoggable is the interface that request options must satisfy
// for structured log output. Domain types satisfy this implicitly
// through duck typing -- no import of the log package is needed.
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

// responseLoggable is the interface that completion responses must satisfy
// for structured log output. Domain types satisfy this implicitly
// through duck typing -- no import of the log package is needed.
type responseLoggable interface {
	LogStopReason() string
	LogContent() string
	LogThinking() string
	LogInputTokens() int
	LogOutputTokens() int
	LogFormatToolCalls(sb *strings.Builder)
}

// responseDevData extends responseLoggable with raw data access for
// JSON dev directory output.
type responseDevData interface {
	LogRawToolCalls() any
	LogRawUsage() any
}
