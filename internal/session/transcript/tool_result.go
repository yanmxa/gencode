package transcript

import "strings"

func HydrateToolResultNodes(sessionID string, nodes []Node, load func(toolCallID string) (string, error)) {
	marker := "\n\n[Full output persisted to blobs/tool-result/" + sessionID + "/"
	for i := range nodes {
		hydrateToolResultBlocks(nodes[i].Content, marker, load)
	}
}

func hydrateToolResultBlocks(blocks []ContentBlock, marker string, load func(toolCallID string) (string, error)) {
	for i := range blocks {
		block := &blocks[i]
		if block.Type != "tool_result" {
			continue
		}
		for j := range block.Content {
			sub := &block.Content[j]
			if sub.Type != "text" {
				continue
			}
			toolCallID, ok := persistedToolResultID(sub.Text, marker)
			if !ok {
				continue
			}
			content, err := load(toolCallID)
			if err == nil {
				sub.Text = content
			}
		}
	}
}

func persistedToolResultID(text, marker string) (string, bool) {
	idx := strings.Index(text, marker)
	if idx < 0 {
		return "", false
	}
	suffix := text[idx+len(marker):]
	end := strings.Index(suffix, "]")
	if end < 0 {
		return "", false
	}
	return suffix[:end], true
}
