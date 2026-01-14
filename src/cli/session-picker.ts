/**
 * Interactive Session Picker - Modern Terminal UI
 * Beautiful session selection with fuzzy search
 */

import type { SessionListItem } from '../session/types.js';
import { theme } from './ui.js';

const icons = {
  search: '⌕',
  selected: '▶',
  session: '◉',
  message: '◯',
  clock: '◷',
  folder: '◇',
  checkmark: '✓',
  arrow: '›',
};

// ============================================================================
// Types
// ============================================================================

interface PickerOptions {
  sessions: SessionListItem[];
  showAllProjects?: boolean;
}

interface PickerResult {
  action: 'select' | 'new' | 'cancel' | 'toggle-all';
  sessionId?: string;
}

// ============================================================================
// Utilities
// ============================================================================

function fuzzyMatch(query: string, text: string): { matches: boolean; score: number } {
  const queryLower = query.toLowerCase();
  const textLower = text.toLowerCase();

  if (!query) return { matches: true, score: 0 };

  let queryIndex = 0;
  let score = 0;
  let lastMatchIndex = -1;

  for (let i = 0; i < textLower.length && queryIndex < queryLower.length; i++) {
    if (textLower[i] === queryLower[queryIndex]) {
      if (lastMatchIndex === i - 1) score += 2;
      if (i === 0 || textLower[i - 1] === ' ' || textLower[i - 1] === '/') score += 3;
      score += 1;
      lastMatchIndex = i;
      queryIndex++;
    }
  }

  return {
    matches: queryIndex === queryLower.length,
    score,
  };
}

function formatRelativeTime(dateStr: string): string {
  const now = Date.now();
  const date = new Date(dateStr).getTime();
  const diff = now - date;

  const seconds = Math.floor(diff / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);

  if (seconds < 60) return 'just now';
  if (minutes < 60) return `${minutes}m ago`;
  if (hours < 24) return `${hours}h ago`;
  if (days < 7) return `${days}d ago`;
  return new Date(dateStr).toLocaleDateString();
}

function highlightMatch(text: string, query: string): string {
  if (!query) return text;

  const queryLower = query.toLowerCase();
  const textLower = text.toLowerCase();
  let result = '';
  let queryIndex = 0;

  for (let i = 0; i < text.length; i++) {
    if (queryIndex < queryLower.length && textLower[i] === queryLower[queryIndex]) {
      result += theme.brand(text[i]);
      queryIndex++;
    } else {
      result += text[i];
    }
  }

  return result;
}

function truncate(str: string, maxLen: number): string {
  if (str.length <= maxLen) return str;
  return str.slice(0, maxLen - 1) + '…';
}

// ============================================================================
// Rendering
// ============================================================================

function render(
  query: string,
  sessions: SessionListItem[],
  selectedIndex: number,
  showAllProjects: boolean,
  termHeight: number
): string[] {
  const lines: string[] = [];
  const termWidth = process.stdout.columns || 80;

  // Header
  lines.push('');
  lines.push(
    theme.brand.bold('  Resume Session') +
    (showAllProjects ? theme.textMuted(' · all projects') : theme.textMuted(' · this project'))
  );
  lines.push('');

  // Search box with nice styling
  const searchIcon = theme.textMuted(icons.search);
  const searchText = query || theme.textMuted('Type to search...');
  const searchLine = `  ${searchIcon} ${searchText}`;
  lines.push(searchLine);
  lines.push(theme.textMuted('  ' + '─'.repeat(Math.min(40, termWidth - 4))));
  lines.push('');

  // Filter and sort sessions
  const filtered = sessions
    .map((s) => {
      const searchText = `${s.title} ${s.preview}`;
      const match = fuzzyMatch(query, searchText);
      return { session: s, ...match };
    })
    .filter((s) => s.matches)
    .sort((a, b) => b.score - a.score);

  // Calculate visible range
  const maxVisible = Math.min(termHeight - 12, 10);
  const startIndex = Math.max(0, selectedIndex - Math.floor(maxVisible / 2));
  const endIndex = Math.min(filtered.length, startIndex + maxVisible);
  const visibleSessions = filtered.slice(startIndex, endIndex);

  // Render sessions
  if (filtered.length === 0) {
    lines.push(theme.textMuted('  No matching sessions found'));
    lines.push('');
  } else {
    for (let i = 0; i < visibleSessions.length; i++) {
      const actualIndex = startIndex + i;
      const isSelected = actualIndex === selectedIndex;
      const s = visibleSessions[i].session;

      // Session item with selection indicator
      const prefix = isSelected
        ? theme.brand(`  ${icons.selected} `)
        : theme.textMuted('    ');

      const titleText = truncate(s.title, 45);
      const title = isSelected
        ? theme.highlight(titleText)
        : highlightMatch(titleText, query);

      // Meta info
      const time = formatRelativeTime(s.updatedAt);
      const msgCount = s.messageCount;
      const meta = theme.textMuted(` · ${msgCount} msgs · ${time}`);

      lines.push(prefix + title + meta);

      // Preview on second line (only for selected item)
      if (isSelected && s.preview) {
        const previewText = truncate(s.preview, Math.min(60, termWidth - 10));
        lines.push(theme.textMuted(`      ${previewText}`));
      }
    }

    // Show "more" indicator if needed
    if (filtered.length > maxVisible) {
      const remaining = filtered.length - visibleSessions.length;
      if (remaining > 0) {
        lines.push(theme.textMuted(`      … ${remaining} more session${remaining > 1 ? 's' : ''}`));
      }
    }
    lines.push('');
  }

  // Footer with keybindings - cleaner styling
  lines.push(theme.textMuted('  ─'.repeat(Math.floor(Math.min(40, termWidth - 4) / 2))));
  lines.push('');

  const keys = [
    `${theme.textSecondary('↑↓')} ${theme.textMuted('navigate')}`,
    `${theme.textSecondary('⏎')} ${theme.textMuted('select')}`,
    `${theme.textSecondary('a')} ${theme.textMuted('toggle all')}`,
    `${theme.textSecondary('esc')} ${theme.textMuted('cancel')}`,
  ];
  lines.push('  ' + keys.join(theme.textMuted(' · ')));

  return lines;
}

// ============================================================================
// Session Picker
// ============================================================================

export async function pickSession(options: PickerOptions): Promise<PickerResult> {
  const { sessions } = options;
  let showAllProjects = options.showAllProjects ?? false;

  if (sessions.length === 0) {
    return { action: 'new' };
  }

  return new Promise((resolve) => {
    let query = '';
    let selectedIndex = 0;
    let lastRenderedLines = 0;

    const termHeight = process.stdout.rows || 24;

    // Enable raw mode for keyboard input
    if (process.stdin.isTTY) {
      process.stdin.setRawMode(true);
    }
    process.stdin.resume();

    const cleanup = () => {
      if (process.stdin.isTTY) {
        process.stdin.setRawMode(false);
      }
      process.stdin.pause();
      process.stdin.removeListener('data', onKeypress);
      // Clear the picker UI
      if (lastRenderedLines > 0) {
        process.stdout.write('\x1b[' + lastRenderedLines + 'A');
        process.stdout.write('\x1b[J');
      }
    };

    const draw = () => {
      // Clear previous output
      if (lastRenderedLines > 0) {
        process.stdout.write('\x1b[' + lastRenderedLines + 'A');
        process.stdout.write('\x1b[J');
      }

      const lines = render(query, sessions, selectedIndex, showAllProjects, termHeight);
      console.log(lines.join('\n'));
      lastRenderedLines = lines.length;
    };

    const getFilteredSessions = () => {
      return sessions
        .map((s) => {
          const searchText = `${s.title} ${s.preview}`;
          const match = fuzzyMatch(query, searchText);
          return { session: s, ...match };
        })
        .filter((s) => s.matches)
        .sort((a, b) => b.score - a.score);
    };

    const onKeypress = (data: Buffer) => {
      const key = data.toString();

      // Escape or Ctrl+C
      if (key === '\x1b' || key === '\x03') {
        cleanup();
        resolve({ action: 'cancel' });
        return;
      }

      // Enter
      if (key === '\r' || key === '\n') {
        const filtered = getFilteredSessions();
        if (filtered.length > 0 && selectedIndex < filtered.length) {
          cleanup();
          resolve({ action: 'select', sessionId: filtered[selectedIndex].session.id });
        }
        return;
      }

      // Arrow up or Ctrl+P
      if (key === '\x1b[A' || key === '\x10') {
        selectedIndex = Math.max(0, selectedIndex - 1);
        draw();
        return;
      }

      // Arrow down or Ctrl+N
      if (key === '\x1b[B' || key === '\x0e') {
        const filtered = getFilteredSessions();
        selectedIndex = Math.min(filtered.length - 1, selectedIndex + 1);
        draw();
        return;
      }

      // Backspace
      if (key === '\x7f' || key === '\b') {
        if (query.length > 0) {
          query = query.slice(0, -1);
          selectedIndex = 0;
          draw();
        }
        return;
      }

      // 'A' or 'a' to toggle all projects (only when no query)
      if ((key === 'A' || key === 'a') && query === '') {
        cleanup();
        resolve({ action: 'toggle-all' });
        return;
      }

      // 'N' or 'n' to start new session (only when no query)
      if ((key === 'N' || key === 'n') && query === '') {
        cleanup();
        resolve({ action: 'new' });
        return;
      }

      // Regular character input
      if (key.length === 1 && key >= ' ') {
        query += key;
        selectedIndex = 0;
        draw();
      }
    };

    process.stdin.on('data', onKeypress);

    // Initial draw
    draw();
  });
}
