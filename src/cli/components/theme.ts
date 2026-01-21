/**
 * Theme Configuration - Claude Code inspired
 */
export const colors = {
  // Brand - Warm orange (Anthropic-inspired, simple and elegant)
  brand: '#FF7B54', // Coral/Orange - warm and approachable
  brandLight: '#FFB38A', // Light coral
  primary: '#FF7B54',

  // Standard status colors (mainstream, widely recognized)
  success: '#22C55E', // Green 500 - standard success
  warning: '#EAB308', // Yellow 500 - standard warning
  error: '#EF4444', // Red 500 - standard error
  info: '#3B82F6', // Blue 500 - standard info

  // Text hierarchy (neutral grays)
  text: '#E2E8F0', // Slate 200 - primary text
  textSecondary: '#94A3B8', // Slate 400 - secondary text
  textMuted: '#64748B', // Slate 500 - muted/hints

  // Tool display - use brand color for consistency
  tool: '#FF7B54', // Same as brand for unified look
  separator: '#334155', // Slate 700 - subtle separator

  // Backgrounds
  inputBg: '#1E293B', // Slate 800 - subtle background

  // Tool UI extensions
  toolHeader: '#3B82F6', // Blue 500 - tool name
  toolBg: '#1E293B', // Slate 800 - background
  toolBorder: '#475569', // Slate 600 - border

  // Status colors (reuse standard)
  statusRunning: '#EAB308', // Yellow 500
  statusSuccess: '#22C55E', // Green 500
  statusError: '#EF4444', // Red 500

  // Permission dialog
  permissionBorder: '#3B82F6', // Blue 500
  optionSelected: '#3B82F6', // Blue 500

  // Diff colors (standard git colors)
  diffAdd: '#22C55E', // Green 500
  diffRemove: '#EF4444', // Red 500
  diffHunk: '#3B82F6', // Blue 500 - hunk headers
};

export const icons = {
  // Message prefixes (Claude Code style)
  userPrompt: '❯', // Chevron for user input
  assistant: '⏺', // Filled circle for assistant (Claude Code uses this)
  toolCall: '⏺', // Filled circle for tool calls
  toolResult: '⎿', // L-connector for tool results
  // Prompt
  prompt: '❯',
  // Status
  success: '✔',
  error: '✖',
  warning: '⚠',
  info: 'ℹ',
  // Tools
  tool: '⚡', // Lightning for tools
  fetch: '●', // Filled circle for fetch (Claude Code style)
  arrow: '→',
  // UI
  thinking: '✱', // Star for thinking state
  cursor: '▋',
  // Selection (single-select)
  radio: '●', // Filled radio for selected
  radioEmpty: '○', // Empty radio for unselected
  // Selection (multi-select)
  checkbox: '☑', // Checked checkbox
  checkboxEmpty: '☐', // Empty checkbox
  // Chip/tag borders (Claude Code style headers)
  chipLeft: '╭─',
  chipRight: '─╮',
  // Box drawing
  boxTop: '╭',
  boxBottom: '╰',
  boxVertical: '│',
  // Tree connectors
  treeEnd: '└', // Tree end connector for tool results
  treeMiddle: '├', // Tree middle connector
  treeLine: '│', // Tree continuation line
  // Mode indicators
  modePlan: '⏸', // Pause for plan mode
  modeAccept: '⏵⏵', // Double play for accept mode

  // Tool-specific icons (Claude Code style - clean Unicode)
  toolBash: '$',
  toolRead: '◇',
  toolWrite: '◆',
  toolEdit: '✎',
  toolGlob: '⦿',
  toolGrep: '⌕',
  toolWeb: '◎',
  toolTodo: '☰',
  toolQuestion: '?',
  toolTask: '⧉',
  toolLsp: '⟡',
  toolNotebook: '▤',

  // Status indicators
  statusCheck: '✓',
  statusCross: '✗',
  statusDot: '●',

  // Box drawing (extended)
  boxTopLeft: '┌',
  boxTopRight: '┐',
  boxBottomLeft: '└',
  boxBottomRight: '┘',
  boxHorizontal: '─',
  boxTeeLeft: '├',
  boxTeeRight: '┤',

  // Rounded box drawing (for permission dialogs)
  roundTopLeft: '╭',
  roundTopRight: '╮',
  roundBottomLeft: '╰',
  roundBottomRight: '╯',

  // Selection indicator
  selectArrow: '▸',

  // Other
  shield: '🛡',
  expand: '▼',
  collapse: '▲',

  // Spinner frames (legacy)
  spinner: ['◐', '◓', '◑', '◒'],

  // GenCode signature pulse animation (unique identity)
  pulseFrames: ['⦿', '⦾', '◉', '◎', '◉', '⦾', '⦿', '○'],
};
