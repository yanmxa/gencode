# Proposal: Interactive CLI UI

- **Proposal ID**: 0038
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement rich interactive CLI UI components including progress indicators, spinners, tables, syntax highlighting, and markdown rendering for an enhanced terminal experience.

## Motivation

Current CLI output is basic:

1. **Plain text**: No formatting or colors
2. **No progress**: Long operations show nothing
3. **No highlighting**: Code not colored
4. **No structure**: Tables are plain text
5. **Limited feedback**: Minimal visual cues

Rich UI improves comprehension and experience.

## Detailed Design

### API Design

```typescript
// src/cli/ui/types.ts
interface Theme {
  primary: string;
  secondary: string;
  success: string;
  warning: string;
  error: string;
  muted: string;
  code: string;
  heading: string;
}

interface SpinnerOptions {
  text: string;
  spinner?: 'dots' | 'line' | 'arc' | 'pulse';
  color?: string;
}

interface ProgressOptions {
  total: number;
  format?: string;
  barWidth?: number;
  showPercent?: boolean;
  showEta?: boolean;
}

interface TableOptions {
  columns: ColumnDef[];
  rows: unknown[][];
  style?: 'simple' | 'box' | 'rounded' | 'minimal';
  maxWidth?: number;
}

interface ColumnDef {
  header: string;
  width?: number | 'auto';
  align?: 'left' | 'center' | 'right';
  formatter?: (value: unknown) => string;
}

interface SyntaxHighlightOptions {
  language: string;
  theme?: 'dark' | 'light' | 'monokai' | 'github';
  lineNumbers?: boolean;
  startLine?: number;
}
```

### UI Components

```typescript
// src/cli/ui/components.ts
import chalk from 'chalk';
import ora from 'ora';
import { marked } from 'marked';
import { highlight } from 'cli-highlight';

class UIComponents {
  private theme: Theme;

  constructor(theme?: Partial<Theme>) {
    this.theme = {
      primary: '#3b82f6',
      secondary: '#6b7280',
      success: '#22c55e',
      warning: '#eab308',
      error: '#ef4444',
      muted: '#9ca3af',
      code: '#a855f7',
      heading: '#f59e0b',
      ...theme
    };
  }

  // Spinner
  spinner(options: SpinnerOptions): Spinner {
    return ora({
      text: options.text,
      spinner: options.spinner || 'dots',
      color: 'cyan'
    });
  }

  // Progress bar
  progress(options: ProgressOptions): ProgressBar {
    return new ProgressBar({
      total: options.total,
      width: options.barWidth || 40,
      format: options.format || '[:bar] :percent :eta',
      complete: chalk.green('█'),
      incomplete: chalk.gray('░')
    });
  }

  // Table
  table(options: TableOptions): string {
    const { columns, rows, style = 'rounded' } = options;

    // Calculate column widths
    const widths = columns.map((col, i) => {
      if (col.width === 'auto') {
        const maxContent = Math.max(
          col.header.length,
          ...rows.map(row => String(row[i]).length)
        );
        return Math.min(maxContent, 40);
      }
      return col.width || 15;
    });

    // Border characters
    const borders = {
      rounded: { tl: '╭', tr: '╮', bl: '╰', br: '╯', h: '─', v: '│', t: '┬', b: '┴', l: '├', r: '┤', c: '┼' },
      box: { tl: '┌', tr: '┐', bl: '└', br: '┘', h: '─', v: '│', t: '┬', b: '┴', l: '├', r: '┤', c: '┼' },
      simple: { tl: '+', tr: '+', bl: '+', br: '+', h: '-', v: '|', t: '+', b: '+', l: '+', r: '+', c: '+' },
      minimal: { tl: '', tr: '', bl: '', br: '', h: '─', v: ' ', t: '', b: '', l: '', r: '', c: '' }
    }[style];

    // Build table
    const lines: string[] = [];

    // Top border
    if (borders.tl) {
      lines.push(borders.tl + widths.map(w => borders.h.repeat(w + 2)).join(borders.t) + borders.tr);
    }

    // Header
    const headerCells = columns.map((col, i) =>
      this.padCell(chalk.bold(col.header), widths[i], col.align)
    );
    lines.push(borders.v + headerCells.map(c => ` ${c} `).join(borders.v) + borders.v);

    // Header separator
    lines.push(borders.l + widths.map(w => borders.h.repeat(w + 2)).join(borders.c) + borders.r);

    // Rows
    for (const row of rows) {
      const cells = row.map((cell, i) => {
        const formatted = columns[i].formatter
          ? columns[i].formatter(cell)
          : String(cell);
        return this.padCell(formatted, widths[i], columns[i].align);
      });
      lines.push(borders.v + cells.map(c => ` ${c} `).join(borders.v) + borders.v);
    }

    // Bottom border
    if (borders.bl) {
      lines.push(borders.bl + widths.map(w => borders.h.repeat(w + 2)).join(borders.b) + borders.br);
    }

    return lines.join('\n');
  }

  // Syntax highlighting
  highlightCode(code: string, options: SyntaxHighlightOptions): string {
    const highlighted = highlight(code, {
      language: options.language,
      theme: {
        keyword: chalk.magenta,
        string: chalk.green,
        number: chalk.cyan,
        comment: chalk.gray,
        function: chalk.yellow,
        class: chalk.blue.bold
      }
    });

    if (options.lineNumbers) {
      const lines = highlighted.split('\n');
      const startLine = options.startLine || 1;
      const padding = String(startLine + lines.length - 1).length;

      return lines.map((line, i) => {
        const lineNum = chalk.gray(String(startLine + i).padStart(padding) + '│');
        return `${lineNum} ${line}`;
      }).join('\n');
    }

    return highlighted;
  }

  // Markdown rendering
  renderMarkdown(markdown: string): string {
    // Custom renderer for terminal
    const renderer = new marked.Renderer();

    renderer.heading = (text, level) => {
      const prefix = '#'.repeat(level);
      return chalk.hex(this.theme.heading).bold(`${prefix} ${text}\n\n`);
    };

    renderer.code = (code, language) => {
      const highlighted = this.highlightCode(code, { language: language || 'text' });
      return `\n${chalk.gray('```' + (language || ''))}\n${highlighted}\n${chalk.gray('```')}\n`;
    };

    renderer.codespan = (code) => {
      return chalk.hex(this.theme.code)(`\`${code}\``);
    };

    renderer.strong = (text) => chalk.bold(text);
    renderer.em = (text) => chalk.italic(text);

    renderer.list = (body, ordered) => {
      return body + '\n';
    };

    renderer.listitem = (text) => {
      return `  • ${text}\n`;
    };

    renderer.link = (href, title, text) => {
      return chalk.blue.underline(text) + chalk.gray(` (${href})`);
    };

    marked.setOptions({ renderer });
    return marked(markdown);
  }

  // Box/panel
  box(content: string, title?: string, style: 'single' | 'double' | 'rounded' = 'rounded'): string {
    const lines = content.split('\n');
    const maxWidth = Math.max(...lines.map(l => l.length), title?.length || 0);

    const borders = {
      rounded: { tl: '╭', tr: '╮', bl: '╰', br: '╯', h: '─', v: '│' },
      single: { tl: '┌', tr: '┐', bl: '└', br: '┘', h: '─', v: '│' },
      double: { tl: '╔', tr: '╗', bl: '╚', br: '╝', h: '═', v: '║' }
    }[style];

    const result: string[] = [];

    // Top with optional title
    if (title) {
      result.push(borders.tl + borders.h + ` ${title} ` + borders.h.repeat(maxWidth - title.length - 1) + borders.tr);
    } else {
      result.push(borders.tl + borders.h.repeat(maxWidth + 2) + borders.tr);
    }

    // Content
    for (const line of lines) {
      result.push(borders.v + ' ' + line.padEnd(maxWidth) + ' ' + borders.v);
    }

    // Bottom
    result.push(borders.bl + borders.h.repeat(maxWidth + 2) + borders.br);

    return result.join('\n');
  }

  private padCell(text: string, width: number, align: string = 'left'): string {
    const stripped = text.replace(/\x1b\[[0-9;]*m/g, '');
    const padding = width - stripped.length;

    if (padding <= 0) return text.slice(0, width);

    switch (align) {
      case 'right':
        return ' '.repeat(padding) + text;
      case 'center':
        const left = Math.floor(padding / 2);
        return ' '.repeat(left) + text + ' '.repeat(padding - left);
      default:
        return text + ' '.repeat(padding);
    }
  }
}

export const ui = new UIComponents();
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/cli/ui/types.ts` | Create | Type definitions |
| `src/cli/ui/components.ts` | Create | UI components |
| `src/cli/ui/themes.ts` | Create | Theme definitions |
| `src/cli/ui/markdown.ts` | Create | Markdown renderer |
| `src/cli/ui/syntax.ts` | Create | Syntax highlighting |
| `package.json` | Modify | Add chalk, ora, marked |

## User Experience

### Progress Bar
```
Installing dependencies...
████████████████████░░░░░░░░░░░░░░░░░░░░ 52% | ETA: 15s
```

### Spinner
```
⠋ Analyzing codebase...
⠙ Analyzing codebase...
⠹ Analyzing codebase...
✓ Analysis complete
```

### Syntax Highlighted Code
```
 1│ import { Agent } from './agent';
 2│
 3│ async function main() {
 4│   const agent = new Agent({
 5│     provider: 'anthropic',
 6│     model: 'claude-sonnet-4'
 7│   });
 8│
 9│   await agent.run();
10│ }
```

### Formatted Table
```
╭────────────────┬──────────┬────────────╮
│ File           │ Status   │ Lines      │
├────────────────┼──────────┼────────────┤
│ src/index.ts   │ Modified │ +45 / -12  │
│ src/agent.ts   │ Modified │ +123 / -67 │
│ src/cli.ts     │ Added    │ +89 / -0   │
╰────────────────┴──────────┴────────────╯
```

## Security Considerations

1. Escape ANSI sequences in user input
2. Limit output width/height
3. Handle terminal resize

## Migration Path

1. **Phase 1**: Spinners and progress
2. **Phase 2**: Tables
3. **Phase 3**: Syntax highlighting
4. **Phase 4**: Markdown rendering
5. **Phase 5**: Themes

## References

- [chalk](https://github.com/chalk/chalk)
- [ora](https://github.com/sindresorhus/ora)
- [marked](https://github.com/markedjs/marked)
- [cli-highlight](https://github.com/felixfbecker/cli-highlight)
