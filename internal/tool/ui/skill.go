package ui

import (
	"fmt"
	"strings"
)

// SkillResultInfo contains skill-specific result metadata for custom rendering
type SkillResultInfo struct {
	SkillName   string // Full skill name (namespace:name)
	ScriptCount int    // Number of scripts in skill
	RefCount    int    // Number of reference files
}

// FormatSkillSummary formats the skill result summary for display
//
// Examples:
//
//	Loaded: git:commit [2 scripts, 1 ref]
//	Loaded: pdf [3 scripts]
//	Loaded: my-skill
func FormatSkillSummary(info *SkillResultInfo) string {
	if info == nil {
		return ""
	}

	var resources []string
	if info.ScriptCount > 0 {
		if info.ScriptCount == 1 {
			resources = append(resources, "1 script")
		} else {
			resources = append(resources, fmt.Sprintf("%d scripts", info.ScriptCount))
		}
	}
	if info.RefCount > 0 {
		if info.RefCount == 1 {
			resources = append(resources, "1 ref")
		} else {
			resources = append(resources, fmt.Sprintf("%d refs", info.RefCount))
		}
	}

	result := fmt.Sprintf("Loaded: %s", info.SkillName)
	if len(resources) > 0 {
		result += fmt.Sprintf(" [%s]", strings.Join(resources, ", "))
	}

	return result
}
