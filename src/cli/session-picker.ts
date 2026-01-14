/**
 * Interactive Session Picker with Fuzzy Search
 */

import * as readline from 'readline';
import type { SessionListItem } from '../session/types.js';
import { colors } from './ui.js';

interface PickerOptions {
  sessions: SessionListItem[];
  showAllProjects?: boolean;
}

interface PickerResult {
  action: 'select' | 'new' | 'cancel' | 'toggle-all';
  sessionId?: string;
}

/**
 * Simple fuzzy match - returns true if all chars in query appear in text in order
 */
function fuzzyMatch(query: string, text: string): { matches: boolean; score: number } {
  const queryLower = query.toLowerCase();
  const textLower = text.toLowerCase();

  if (!query) return { matches: true, score: 0 };

  let queryIndex = 0;
  let score = 0;
  let lastMatchIndex = -1;

  for (let i = 0; i < textLower.length && queryIndex < queryLower.length; i++) {
    if (textLower[i] === queryLower[queryIndex]) {
      // Bonus for consecutive matches
      if (lastMatchIndex === i - 1) score += 2;
      // Bonus for matching at word start
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

/**
 * Format relative time
 */
function formatRelativeTime(dateStr: string): string {
  const now = Date.now();
  const date = new Date(dateStr).getTime();
  const diff = now - date;

  const seconds = Math.floor(diff / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);

  if (seconds < 60) return `${seconds} seconds ago`;
  if (minutes < 60) return `${minutes} minute${minutes > 1 ? 's' : ''} ago`;
  if (hours < 24) return `${hours} hour${hours > 1 ? 's' : ''} ago`;
  if (days < 7) return `${days} day${days > 1 ? 's' : ''} ago`;
  return new Date(dateStr).toLocaleDateString();
}

/**
 * Highlight matching characters in text
 */
function highlightMatch(text: string, query: string): string {
  if (!query) return text;

  const queryLower = query.toLowerCase();
  const textLower = text.toLowerCase();
  let result = '';
  let queryIndex = 0;

  for (let i = 0; i < text.length; i++) {
    if (queryIndex < queryLower.length && textLower[i] === queryLower[queryIndex]) {
      result += colors.primary(text[i]);
      queryIndex++;
    } else {
      result += text[i];
    }
  }

  return result;
}

/**
 * Render the picker UI
 */
function render(
  query: string,
  sessions: SessionListItem[],
  selectedIndex: number,
  showAllProjects: boolean,
  termHeight: number
): string[] {
  const lines: string[] = [];

  // Header
  lines.push('');
  lines.push(colors.info('Resume Session') + (showAllProjects ? colors.muted(' (all projects)') : ''));
  lines.push('');

  // Search box
  const searchBox = colors.muted('› ') + (query || colors.muted('Search...'));
  lines.push(searchBox);
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
  const maxVisible = Math.min(termHeight - 10, 15);
  const startIndex = Math.max(0, selectedIndex - Math.floor(maxVisible / 2));
  const endIndex = Math.min(filtered.length, startIndex + maxVisible);
  const visibleSessions = filtered.slice(startIndex, endIndex);

  // Render sessions
  if (filtered.length === 0) {
    lines.push(colors.muted('  No matching sessions'));
  } else {
    visibleSessions.forEach((item, i) => {
      const actualIndex = startIndex + i;
      const isSelected = actualIndex === selectedIndex;
      const s = item.session;

      const prefix = isSelected ? colors.primary('› ▶ ') : '    ';
      const title = highlightMatch(s.title.slice(0, 60), query);
      const relTime = formatRelativeTime(s.updatedAt);
      const meta = colors.muted(` · ${s.messageCount} messages · ${relTime}`);

      lines.push(prefix + (isSelected ? colors.highlight(s.title.slice(0, 60)) : title) + meta);

      if (s.preview) {
        const previewText = s.preview.slice(0, 80);
        const preview = isSelected
          ? colors.muted('    ' + previewText)
          : colors.muted('    ' + highlightMatch(previewText, query));
        lines.push(preview);
      }
    });

    if (filtered.length > maxVisible) {
      lines.push(colors.muted(`    ... and ${filtered.length - maxVisible} more sessions`));
    }
  }

  // Footer with keybindings
  lines.push('');
  lines.push(
    colors.muted('A') + ' to show all projects · ' +
    colors.muted('↑↓') + ' to navigate · ' +
    colors.muted('Enter') + ' to select · ' +
    colors.muted('Esc') + ' to cancel'
  );

  return lines;
}

/**
 * Interactive session picker
 */
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

    // Enable raw mode
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
      process.stdout.write('\x1b[' + lastRenderedLines + 'A'); // Move up
      process.stdout.write('\x1b[J'); // Clear to end
    };

    const draw = () => {
      // Clear previous output
      if (lastRenderedLines > 0) {
        process.stdout.write('\x1b[' + lastRenderedLines + 'A'); // Move up
        process.stdout.write('\x1b[J'); // Clear to end
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

      // Escape
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

      // Arrow up
      if (key === '\x1b[A') {
        const filtered = getFilteredSessions();
        selectedIndex = Math.max(0, selectedIndex - 1);
        draw();
        return;
      }

      // Arrow down
      if (key === '\x1b[B') {
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

      // 'A' or 'a' to toggle all projects
      if ((key === 'A' || key === 'a') && query === '') {
        cleanup();
        resolve({ action: 'toggle-all' });
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
