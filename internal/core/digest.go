package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// SHA256Digest returns "sha256:" + lowercase hex of the SHA-256 sum of b.
// The prefix makes algorithm choice explicit on the wire.
func SHA256Digest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ToolsDigest canonicalizes a tool schema list (sort by Name, marshal each)
// and returns its sha256. Stable across runs as long as schemas are stable.
func ToolsDigest(schemas []ToolSchema) string {
	if len(schemas) == 0 {
		return SHA256Digest(nil)
	}
	sorted := make([]ToolSchema, len(schemas))
	copy(sorted, schemas)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	b, err := json.Marshal(sorted)
	if err != nil {
		// Marshal can only fail on unsupported types; ToolSchema fields are
		// JSON-safe. Fall back to a digest of the names so the value is still
		// stable rather than empty.
		names := make([]string, len(sorted))
		for i, s := range sorted {
			names[i] = s.Name
		}
		b, _ = json.Marshal(names)
	}
	return SHA256Digest(b)
}

// MessageIDs extracts non-empty message IDs from the conversation snapshot,
// in send order. Empty IDs (legacy data) are skipped rather than padded.
func MessageIDs(msgs []Message) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if m.ID != "" {
			out = append(out, m.ID)
		}
	}
	return out
}
