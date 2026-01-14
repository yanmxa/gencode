/**
 * CLI UI Components - Modern Terminal Interface
 * Inspired by Claude Code and OpenCode design patterns
 */

import chalk from 'chalk';
import ora from 'ora';
import * as readline from 'readline';

// ============================================================================
// Theme Configuration
// ============================================================================

export const theme = {
  // Brand colors
  brand: chalk.hex('#7C3AED'),       // Purple - primary brand
  brandLight: chalk.hex('#A78BFA'),  // Light purple
  brandDim: chalk.hex('#6D28D9'),    // Dark purple

  // Semantic colors
  primary: chalk.hex('#06B6D4'),     // Cyan - primary actions
  success: chalk.hex('#10B981'),     // Green - success
  warning: chalk.hex('#F59E0B'),     // Amber - warnings
  error: chalk.hex('#EF4444'),       // Red - errors
  info: chalk.hex('#3B82F6'),        // Blue - info

  // Neutral colors
  text: chalk.hex('#F8FAFC'),        // White text
  textSecondary: chalk.hex('#94A3B8'), // Gray text
  textMuted: chalk.hex('#64748B'),   // Muted text
  dim: chalk.dim,
  bold: chalk.bold,

  // Special
  highlight: chalk.bold.hex('#F8FAFC'),
  link: chalk.hex('#3B82F6').underline,
  code: chalk.hex('#A78BFA'),
  tool: chalk.hex('#F472B6'),        // Pink for tools
};

// Export for backward compatibility
export const colors = {
  primary: theme.primary,
  secondary: theme.textSecondary,
  success: theme.success,
  error: theme.error,
  warning: theme.warning,
  info: theme.info,
  muted: theme.textMuted,
  highlight: theme.highlight,
  tool: theme.tool,
  user: theme.brand,
  assistant: theme.success,
};

// ============================================================================
// Unicode Characters
// ============================================================================

const icons = {
  // Status indicators
  success: '✓',
  error: '✗',
  warning: '⚠',
  info: 'ℹ',

  // Prompt and navigation
  prompt: '❯',
  promptAlt: '›',
  arrowRight: '→',
  arrowLeft: '←',
  arrowUp: '↑',
  arrowDown: '↓',

  // UI elements
  bullet: '•',
  ellipsis: '…',
  separator: '│',
  cornerTopLeft: '╭',
  cornerTopRight: '╮',
  cornerBottomLeft: '╰',
  cornerBottomRight: '╯',
  horizontal: '─',
  vertical: '│',

  // Agent/tool
  tool: '⚙',
  thinking: '◐',
  user: '▶',
  assistant: '◀',
  session: '◉',
};

// ============================================================================
// Layout Utilities
// ============================================================================

function getTerminalWidth(): number {
  return process.stdout.columns || 80;
}

function truncate(str: string, maxLength: number): string {
  if (str.length <= maxLength) return str;
  return str.slice(0, maxLength - 1) + icons.ellipsis;
}

function padRight(str: string, length: number): string {
  const visibleLength = stripAnsi(str).length;
  return str + ' '.repeat(Math.max(0, length - visibleLength));
}

function stripAnsi(str: string): string {
  // eslint-disable-next-line no-control-regex
  return str.replace(/\x1B\[[0-9;]*[a-zA-Z]/g, '');
}

// ============================================================================
// Box Drawing
// ============================================================================

const box = {
  topLeft: icons.cornerTopLeft,
  topRight: icons.cornerTopRight,
  bottomLeft: icons.cornerBottomLeft,
  bottomRight: icons.cornerBottomRight,
  horizontal: icons.horizontal,
  vertical: icons.vertical,
};

export function drawBox(title: string, content: string, color = theme.primary): string {
  const lines = content.split('\n');
  const maxWidth = Math.max(title.length + 4, ...lines.map((l) => stripAnsi(l).length)) + 2;
  const width = Math.min(maxWidth, getTerminalWidth() - 4);

  const topLine = color(
    box.topLeft + box.horizontal + ' ' + title + ' ' + box.horizontal.repeat(width - title.length - 4) + box.topRight
  );

  const contentLines = lines.map((line) => {
    const stripped = stripAnsi(line);
    const truncated = stripped.length > width - 2 ? truncate(stripped, width - 2) : line;
    const padding = ' '.repeat(Math.max(0, width - 2 - stripAnsi(truncated).length));
    return color(box.vertical) + ' ' + truncated + padding + color(box.vertical);
  });

  const bottomLine = color(box.bottomLeft + box.horizontal.repeat(width) + box.bottomRight);

  return [topLine, ...contentLines, bottomLine].join('\n');
}

// ============================================================================
// Spinners
// ============================================================================

export function createSpinner(text: string) {
  return ora({
    text: theme.textSecondary(text),
    spinner: {
      interval: 80,
      frames: ['⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'],
    },
    color: 'magenta',
  });
}

export function createThinkingSpinner(text = 'Thinking') {
  return ora({
    text: theme.brandLight(text),
    spinner: {
      interval: 120,
      frames: ['◐', '◓', '◑', '◒'],
    },
    color: 'magenta',
  });
}

// ============================================================================
// Header & Branding
// ============================================================================

export function printHeader(): void {
  const width = Math.min(getTerminalWidth(), 60);

  console.log();
  console.log(theme.brand(box.topLeft + box.horizontal.repeat(width - 2) + box.topRight));

  const title = theme.bold('mycode');
  const subtitle = theme.textMuted('AI-Powered Coding Assistant');
  const titleLine = `${box.vertical}  ${title} ${theme.textMuted('·')} ${subtitle}`;
  const padding = ' '.repeat(Math.max(0, width - 2 - stripAnsi(titleLine).length + 1));
  console.log(theme.brand(box.vertical) + `  ${title} ${theme.textMuted('·')} ${subtitle}` + padding + theme.brand(box.vertical));

  console.log(theme.brand(box.bottomLeft + box.horizontal.repeat(width - 2) + box.bottomRight));
  console.log();
}

export function printCompactHeader(provider: string, model: string, cwd: string): void {
  const width = Math.min(getTerminalWidth(), 80);

  console.log();
  console.log(theme.brand.bold('  mycode'));
  console.log(theme.textMuted('  ─'.repeat(Math.floor(width / 2) - 2)));

  const providerInfo = `${theme.textMuted('Provider:')} ${theme.primary(provider)} ${theme.textMuted('·')} ${theme.textMuted('Model:')} ${theme.primary(model)}`;
  console.log('  ' + providerInfo);

  const cwdShort = cwd.length > 50 ? '...' + cwd.slice(-47) : cwd;
  console.log(`  ${theme.textMuted('Directory:')} ${theme.textSecondary(cwdShort)}`);
  console.log();
}

export function printSeparator(): void {
  const width = Math.min(getTerminalWidth(), 60);
  console.log(theme.textMuted('─'.repeat(width)));
}

export function printLightSeparator(): void {
  const width = Math.min(getTerminalWidth(), 40);
  console.log(theme.dim('·'.repeat(width)));
}

// ============================================================================
// Messages
// ============================================================================

export function printUserMessage(message: string): void {
  console.log();
  console.log(theme.brand(`${icons.user} You`));
  const lines = message.split('\n');
  for (const line of lines) {
    console.log(theme.text('  ' + line));
  }
}

export function printAssistantMessage(message: string): void {
  console.log();
  console.log(theme.success(`${icons.assistant} Assistant`));
  const lines = message.split('\n');
  for (const line of lines) {
    console.log('  ' + line);
  }
}

export function printToolCall(name: string, input: unknown): void {
  console.log();
  console.log(theme.tool(`${icons.tool} `) + theme.highlight(name));

  const inputStr = JSON.stringify(input, null, 2);
  const lines = inputStr.split('\n').slice(0, 8);
  for (const line of lines) {
    console.log(theme.textMuted('  ' + line));
  }
  if (inputStr.split('\n').length > 8) {
    console.log(theme.textMuted('  ' + icons.ellipsis));
  }
}

export function printToolResult(name: string, success: boolean, output: string): void {
  const status = success
    ? theme.success(icons.success)
    : theme.error(icons.error);

  console.log(`${status} ${theme.textMuted(name)}`);

  if (output && output.length > 0) {
    const lines = output.split('\n').slice(0, 10);
    for (const line of lines) {
      const truncatedLine = truncate(line, getTerminalWidth() - 6);
      console.log(theme.textMuted('  │ ') + theme.dim(truncatedLine));
    }
    if (output.split('\n').length > 10) {
      console.log(theme.textMuted('  │ ' + icons.ellipsis + ' (truncated)'));
    }
  }
  console.log();
}

// ============================================================================
// Status Messages
// ============================================================================

export function printError(message: string): void {
  console.log();
  console.log(theme.error(`${icons.error} Error: `) + theme.text(message));
}

export function printSuccess(message: string): void {
  console.log(theme.success(`${icons.success} `) + theme.text(message));
}

export function printWarning(message: string): void {
  console.log(theme.warning(`${icons.warning} `) + theme.text(message));
}

export function printInfo(message: string): void {
  console.log(theme.info(`${icons.info} `) + theme.textSecondary(message));
}

// ============================================================================
// Prompts
// ============================================================================

export function getPromptSymbol(): string {
  return theme.brand(icons.prompt) + ' ';
}

export async function promptConfirm(message: string): Promise<boolean> {
  const rl = readline.createInterface({
    input: process.stdin,
    output: process.stdout,
  });

  return new Promise((resolve) => {
    const prompt = theme.warning(icons.promptAlt + ' ') + message + theme.textMuted(' (y/n) ');
    rl.question(prompt, (answer) => {
      rl.close();
      resolve(answer.toLowerCase() === 'y' || answer.toLowerCase() === 'yes');
    });
  });
}

export async function promptInput(prompt: string): Promise<string> {
  const rl = readline.createInterface({
    input: process.stdin,
    output: process.stdout,
  });

  return new Promise((resolve) => {
    rl.question(getPromptSymbol() + prompt, (answer) => {
      rl.close();
      resolve(answer);
    });
  });
}

// ============================================================================
// Progress
// ============================================================================

export function printProgress(current: number, total: number, label: string): void {
  const width = 25;
  const filled = Math.round((current / total) * width);
  const empty = width - filled;
  const bar = theme.brand('█'.repeat(filled)) + theme.textMuted('░'.repeat(empty));
  const percent = Math.round((current / total) * 100);

  process.stdout.write(`\r${bar} ${theme.text(percent + '%')} ${theme.textMuted(label)}`);
}

// ============================================================================
// Tables
// ============================================================================

export function printTable(headers: string[], rows: string[][]): void {
  const widths = headers.map((h, i) =>
    Math.max(h.length, ...rows.map((r) => (r[i] || '').length))
  );

  // Header
  const headerRow = headers.map((h, i) => padRight(h, widths[i])).join(theme.textMuted(' │ '));
  console.log(theme.highlight(headerRow));

  // Separator
  const separator = widths.map((w) => '─'.repeat(w)).join('─┼─');
  console.log(theme.textMuted(separator));

  // Rows
  for (const row of rows) {
    const cells = row.map((cell, i) => padRight(cell || '', widths[i]));
    console.log(cells.join(theme.textMuted(' │ ')));
  }
}

// ============================================================================
// Session Display
// ============================================================================

export function printSessionInfo(sessionId: string, messageCount: number): void {
  console.log();
  console.log(theme.brand(`${icons.session} Session`));
  console.log(`  ${theme.textMuted('ID:')} ${theme.primary(sessionId.slice(0, 12))}`);
  console.log(`  ${theme.textMuted('Messages:')} ${theme.text(String(messageCount))}`);
}

// ============================================================================
// Help Display
// ============================================================================

export function printHelp(): void {
  console.log();
  console.log(theme.highlight('Commands'));
  console.log();

  const commands = [
    { cmd: '/sessions', desc: 'List sessions (--all for all projects)' },
    { cmd: '/resume', desc: 'Resume a session (latest if no arg)' },
    { cmd: '/new', desc: 'Start a new session' },
    { cmd: '/fork', desc: 'Fork current session' },
    { cmd: '/delete', desc: 'Delete a session' },
    { cmd: '/save', desc: 'Save current session' },
    { cmd: '/info', desc: 'Show current session info' },
    { cmd: '/clear', desc: 'Clear conversation' },
    { cmd: '/help', desc: 'Show this help' },
  ];

  const maxCmdLen = Math.max(...commands.map(c => c.cmd.length));

  for (const { cmd, desc } of commands) {
    console.log(`  ${theme.primary(cmd.padEnd(maxCmdLen + 2))}${theme.textMuted(desc)}`);
  }

  console.log();
  console.log(theme.textMuted('  Type exit or quit to leave'));
}

// ============================================================================
// Welcome Message
// ============================================================================

export function printWelcome(): void {
  console.log();
  console.log(theme.textSecondary('  Type your message to start. Use /help for commands.'));
  console.log(theme.textMuted('  Use -c to continue last session, -r to select.'));
}

// ============================================================================
// Permission Prompt
// ============================================================================

export function printPermissionRequest(tool: string, input: unknown): void {
  console.log();
  console.log(theme.warning(`${icons.warning} Permission Required`));
  console.log(`  ${theme.textMuted('Tool:')} ${theme.highlight(tool)}`);

  const inputStr = JSON.stringify(input, null, 2);
  const lines = inputStr.split('\n').slice(0, 5);
  console.log(theme.textMuted('  Input:'));
  for (const line of lines) {
    console.log(theme.textMuted('    ' + line));
  }
  if (inputStr.split('\n').length > 5) {
    console.log(theme.textMuted('    ' + icons.ellipsis));
  }
}
