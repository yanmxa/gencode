/**
 * Theme Configuration - Claude Code inspired
 */
export const colors = {
  brand: '#818CF8', // Indigo 400
  brandLight: '#A5B4FC', // Indigo 300
  primary: '#818CF8',
  success: '#34D399', // Emerald 400
  warning: '#FBBF24', // Amber 400
  error: '#F87171', // Red 400
  info: '#60A5FA', // Blue 400
  text: '#F1F5F9', // Slate 100
  textSecondary: '#94A3B8', // Slate 400
  textMuted: '#64748B', // Slate 500
  tool: '#C084FC', // Purple 400
  separator: '#1E293B', // Slate 800
  inputBg: '#111827', // Gray 900 - subtle background for user input

  // Tool UI extensions
  toolHeader: '#60A5FA', // Blue 400 - tool name
  toolBg: '#1E293B', // Slate 800 - background
  toolBorder: '#334155', // Slate 700 - border

  // Status colors
  statusRunning: '#FBBF24', // Amber 400 - running
  statusSuccess: '#34D399', // Emerald 400 - success
  statusError: '#F87171', // Red 400 - error

  // Permission dialog
  permissionBorder: '#3B82F6', // Blue 500
  optionSelected: '#3B82F6', // Blue 500

  // Diff colors
  diffAdd: '#22C55E', // Green 500
  diffRemove: '#EF4444', // Red 500
  diffHunk: '#06B6D4', // Cyan 500
};

export const icons = {
  // Message prefixes (Claude Code style)
  userPrompt: '❯', // Chevron for user input
  assistant: '●', // Filled circle for assistant
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

  // Tool-specific icons (terminal style)
  toolBash: '[$]',
  toolRead: '[R]',
  toolWrite: '[W]',
  toolEdit: '[E]',
  toolGlob: '[G]',
  toolGrep: '[S]',
  toolWeb: '[W]',
  toolTodo: '[T]',
  toolQuestion: '[?]',

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

  // Spinner frames
  spinner: ['◐', '◓', '◑', '◒'],
};
