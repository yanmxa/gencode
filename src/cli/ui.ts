/**
 * CLI UI Components - Beautiful terminal output
 */

import chalk from 'chalk';
import ora from 'ora';
import * as readline from 'readline';

// ============================================================================
// Colors & Styles
// ============================================================================

export const colors = {
  primary: chalk.cyan,
  secondary: chalk.gray,
  success: chalk.green,
  error: chalk.red,
  warning: chalk.yellow,
  info: chalk.blue,
  muted: chalk.dim,
  highlight: chalk.bold.white,
  tool: chalk.magenta,
  user: chalk.blue,
  assistant: chalk.green,
};

// ============================================================================
// Box Drawing
// ============================================================================

const box = {
  topLeft: '╭',
  topRight: '╮',
  bottomLeft: '╰',
  bottomRight: '╯',
  horizontal: '─',
  vertical: '│',
};

export function drawBox(title: string, content: string, color = colors.primary): string {
  const lines = content.split('\n');
  const maxWidth = Math.max(title.length + 4, ...lines.map((l) => l.length)) + 2;
  const width = Math.min(maxWidth, process.stdout.columns - 4 || 80);

  const topLine = color(
    box.topLeft + box.horizontal + ' ' + title + ' ' + box.horizontal.repeat(width - title.length - 4) + box.topRight
  );

  const contentLines = lines.map((line) => {
    const truncated = line.slice(0, width - 2);
    const padding = ' '.repeat(Math.max(0, width - 2 - truncated.length));
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
    text: colors.secondary(text),
    spinner: 'dots',
    color: 'cyan',
  });
}

// ============================================================================
// Headers & Separators
// ============================================================================

export function printHeader(): void {
  const logo = `
  ${chalk.cyan('╭─────────────────────────────────────╮')}
  ${chalk.cyan('│')}  ${chalk.bold.white('MyCode')} ${chalk.dim('- Multi-LLM Agent CLI')}    ${chalk.cyan('│')}
  ${chalk.cyan('╰─────────────────────────────────────╯')}
`;
  console.log(logo);
}

export function printSeparator(): void {
  const width = process.stdout.columns || 80;
  console.log(colors.muted('─'.repeat(Math.min(width - 2, 60))));
}

// ============================================================================
// Messages
// ============================================================================

export function printUserMessage(message: string): void {
  console.log();
  console.log(colors.user('▶ You:'));
  console.log(colors.highlight('  ' + message));
  console.log();
}

export function printAssistantMessage(message: string): void {
  console.log(colors.assistant('◀ Assistant:'));
  const lines = message.split('\n');
  for (const line of lines) {
    console.log('  ' + line);
  }
}

export function printToolCall(name: string, input: unknown): void {
  console.log();
  console.log(colors.tool('⚙ Tool: ') + colors.highlight(name));
  const inputStr = JSON.stringify(input, null, 2);
  const lines = inputStr.split('\n').slice(0, 10);
  for (const line of lines) {
    console.log(colors.muted('  ' + line));
  }
  if (inputStr.split('\n').length > 10) {
    console.log(colors.muted('  ...'));
  }
}

export function printToolResult(name: string, success: boolean, output: string): void {
  const status = success ? colors.success('✓') : colors.error('✗');
  console.log(status + colors.muted(` ${name} completed`));

  if (output && output.length > 0) {
    const lines = output.split('\n').slice(0, 15);
    for (const line of lines) {
      console.log(colors.muted('  │ ') + line.slice(0, 100));
    }
    if (output.split('\n').length > 15) {
      console.log(colors.muted('  │ ... (truncated)'));
    }
  }
  console.log();
}

export function printError(message: string): void {
  console.log();
  console.log(colors.error('✗ Error: ') + message);
  console.log();
}

export function printSuccess(message: string): void {
  console.log(colors.success('✓ ') + message);
}

export function printInfo(message: string): void {
  console.log(colors.info('ℹ ') + message);
}

// ============================================================================
// Prompts
// ============================================================================

export async function promptConfirm(message: string): Promise<boolean> {
  const rl = readline.createInterface({
    input: process.stdin,
    output: process.stdout,
  });

  return new Promise((resolve) => {
    rl.question(colors.warning('? ') + message + colors.muted(' (y/n) '), (answer) => {
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
    rl.question(colors.primary('❯ ') + prompt, (answer) => {
      rl.close();
      resolve(answer);
    });
  });
}

// ============================================================================
// Progress
// ============================================================================

export function printProgress(current: number, total: number, label: string): void {
  const width = 30;
  const filled = Math.round((current / total) * width);
  const empty = width - filled;
  const bar = colors.primary('█'.repeat(filled)) + colors.muted('░'.repeat(empty));
  const percent = Math.round((current / total) * 100);

  process.stdout.write(`\r${bar} ${percent}% ${colors.muted(label)}`);
}

// ============================================================================
// Tables
// ============================================================================

export function printTable(headers: string[], rows: string[][]): void {
  const widths = headers.map((h, i) => Math.max(h.length, ...rows.map((r) => (r[i] || '').length)));

  const headerRow = headers.map((h, i) => h.padEnd(widths[i])).join(colors.muted(' │ '));
  console.log(colors.highlight(headerRow));

  const separator = widths.map((w) => '─'.repeat(w)).join(colors.muted('─┼─'));
  console.log(colors.muted(separator));

  for (const row of rows) {
    const rowStr = row.map((cell, i) => (cell || '').padEnd(widths[i])).join(colors.muted(' │ '));
    console.log(rowStr);
  }
}
