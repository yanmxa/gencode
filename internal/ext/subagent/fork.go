package subagent

import "github.com/yanmxa/gencode/internal/message"

// MaxForkDepth is the maximum number of nested forks allowed.
// This is separate from MaxAgentNestingDepth because forked agents carry
// the full parent conversation, making context growth much faster.
const MaxForkDepth = 2

// CountForkDepth estimates fork depth by counting system prompt markers
// that indicate the conversation was inherited via fork. Each fork
// appends a system-level note to the conversation.
func CountForkDepth(messages []message.Message) int {
	depth := 0
	for _, msg := range messages {
		if msg.Role == "system" && isForkNote(msg.Content) {
			depth++
		}
	}
	return depth
}

const forkNote = "[This conversation was forked from a parent context]"

func isForkNote(content string) bool {
	return content == forkNote
}

// PrepareForkedMessages takes parent messages and prepares them for the
// forked agent. It appends a fork note so nested forks can be detected.
func PrepareForkedMessages(parentMessages []message.Message) []message.Message {
	if len(parentMessages) == 0 {
		return nil
	}
	forked := make([]message.Message, len(parentMessages), len(parentMessages)+1)
	copy(forked, parentMessages)
	forked = append(forked, message.Message{
		Role:    "system",
		Content: forkNote,
	})
	return forked
}
