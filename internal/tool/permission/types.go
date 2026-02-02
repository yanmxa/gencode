package permission

// PermissionRequest represents a request for user permission before executing a tool
type PermissionRequest struct {
	ID          string         // Unique request ID
	ToolName    string         // Name of the tool requesting permission
	FilePath    string         // File path being modified
	Description string         // Human-readable description of the action
	DiffMeta    *DiffMetadata  // Diff metadata (for Edit/Write tools)
	BashMeta    *BashMetadata  // Bash metadata (for Bash tool)
	SkillMeta   *SkillMetadata // Skill metadata (for Skill tool)
}

// DiffMetadata contains diff information for file modifications
type DiffMetadata struct {
	OldContent   string     // Original file content
	NewContent   string     // New file content after modification
	UnifiedDiff  string     // Unified diff format
	Lines        []DiffLine // Parsed diff lines
	IsNewFile    bool       // Whether this is a new file creation
	PreviewMode  bool       // True to show content preview instead of diff (for Write tool)
	AddedCount   int        // Number of lines added
	RemovedCount int        // Number of lines removed
}

// BashMetadata contains metadata for Bash command permission requests
type BashMetadata struct {
	Command       string // The command to execute
	Description   string // Optional description of what the command does
	RunBackground bool   // Whether to run in background
	LineCount     int    // Number of lines in the command
}

// SkillMetadata contains metadata for Skill permission requests
type SkillMetadata struct {
	SkillName   string   // Full skill name (namespace:name)
	Description string   // Skill description
	Args        string   // Optional arguments passed to the skill
	ScriptCount int      // Number of available scripts
	RefCount    int      // Number of reference files
	Scripts     []string // Script file names
	References  []string // Reference file names
}

// DiffLine represents a single line in a diff
type DiffLine struct {
	Type      DiffLineType // Type of diff line
	Content   string       // Line content (without +/- prefix)
	OldLineNo int          // Line number in old file (0 if not applicable)
	NewLineNo int          // Line number in new file (0 if not applicable)
}

// DiffLineType represents the type of a diff line
type DiffLineType int

const (
	DiffLineContext  DiffLineType = iota // Unchanged line (context)
	DiffLineAdded                        // Added line (+)
	DiffLineRemoved                      // Removed line (-)
	DiffLineHunk                         // Hunk header (@@ ... @@)
	DiffLineMetadata                     // Metadata line (\ No newline at end of file)
)

// PermissionResponse represents the user's response to a permission request
type PermissionResponse struct {
	RequestID string // ID of the original request
	Approved  bool   // Whether the action was approved
}
