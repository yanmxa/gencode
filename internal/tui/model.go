// Core data types: the model struct, chatMessage, operationMode, and stream message types.
package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/glamour"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tui/suggest"
)

type operationMode int

const (
	modeNormal operationMode = iota
	modeAutoAccept
	modePlan
)

func (m operationMode) Next() operationMode {
	return (m + 1) % 3
}

const (
	roleUser      = "user"
	roleAssistant = "assistant"
	roleNotice    = "notice"
)

type chatMessage struct {
	role              string
	content           string
	thinking          string
	images            []message.ImageData
	toolCalls         []message.ToolCall
	toolCallsExpanded bool
	toolResult        *message.ToolResult
	toolName          string
	expanded          bool
	renderedInline    bool
	isSummary         bool
	summaryCount      int
}

type (
	streamChunkMsg struct {
		text             string
		thinking         string
		done             bool
		err              error
		toolCalls        []message.ToolCall
		stopReason       string
		buildingToolName string
		usage            *message.Usage
	}
	streamDoneMsg     struct{}
	streamContinueMsg struct {
		messages []message.Message
		modelID  string
	}
	EditorFinishedMsg struct {
		Err error
	}
)

type compactState struct {
	active       bool
	focus        string
	autoContinue bool
}

func (c *compactState) Reset() {
	c.active = false
	c.focus = ""
	c.autoContinue = false
}

type toolExecState struct {
	pendingCalls    []message.ToolCall
	currentIdx      int
	parallel        bool
	parallelResults map[int]message.ToolResult
	parallelCount   int
}

func (t *toolExecState) Reset() {
	t.pendingCalls = nil
	t.currentIdx = 0
	t.parallel = false
	t.parallelResults = nil
	t.parallelCount = 0
}

type model struct {
	textarea     textarea.Model
	spinner      spinner.Model
	messages     []chatMessage
	llmProvider  provider.LLMProvider
	store        *provider.Store
	currentModel *provider.CurrentModelInfo
	streaming    bool
	streamChan   <-chan message.StreamChunk
	cancelFunc   context.CancelFunc
	width        int
	height       int
	ready        bool
	inputHistory []string
	historyIndex int
	tempInput    string
	cwd          string

	mdRenderer *glamour.TermRenderer

	lastCtrlOTime   time.Time

	suggestions suggest.State

	selector       SelectorState
	memorySelector MemorySelectorState

	permissionPrompt *PermissionPrompt
	toolExec         toolExecState

	settings           *config.Settings
	sessionPermissions *config.SessionPermissions

	questionPrompt  *QuestionPrompt
	pendingQuestion *tool.QuestionRequest

	enterPlanPrompt *EnterPlanPrompt

	planMode   bool
	planTask   string
	planPrompt *PlanPrompt
	planStore  *plan.Store

	operationMode operationMode

	buildingToolName string

	disabledTools  map[string]bool
	toolSelector   ToolSelectorState
	mcpSelector    MCPSelectorState
	pluginSelector PluginSelectorState

	skillSelector            SkillSelectorState
	pendingSkillInstructions string
	pendingSkillArgs         string

	agentSelector AgentSelectorState

	taskProgress map[int][]string // per-agent progress indexed by tool call position

	lastInputTokens  int
	lastOutputTokens int

	fetchingTokenLimits bool

	compact compactState

	sessionStore           *session.Store
	currentSessionID       string
	sessionSelector        SessionSelectorState
	pendingSessionSelector bool

	editingMemoryFile string

	hookEngine *hooks.Engine

	mcpRegistry *mcp.Registry

	loop *core.Loop

	committedCount     int
	pendingClearScreen bool

	cachedMemory string

	pendingImages   []message.ImageData
	imageSelectMode bool
	selectedImageIdx int
}