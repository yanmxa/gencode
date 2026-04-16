package toolresult

// SkillResultInfo contains skill-specific result metadata for custom rendering
type SkillResultInfo struct {
	SkillName   string // Full skill name (namespace:name)
	ScriptCount int    // Number of scripts in skill
	RefCount    int    // Number of reference files
}
