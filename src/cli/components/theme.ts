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
};
