/**
 * Simple Terminal Markdown Renderer
 * Uses marked for parsing, chalk for ANSI colors
 */

import { marked, type Tokens, type RendererObject, type Token } from 'marked';
import chalk from 'chalk';

// Helper type for renderer context with parser access
type RendererContext = {
  parser?: {
    parseInline(tokens: Token[]): string;
    parse(tokens: Token[]): string;
  }
};

// Helper to parse inline tokens from renderer context
function parseInline(ctx: unknown, tokens: Token[], fallback: string): string {
  const context = ctx as RendererContext;
  return context.parser?.parseInline(tokens) ?? fallback;
}

// Helper to parse block tokens (like list item content)
function parseTokens(ctx: unknown, tokens: Token[], fallback: string): string {
  const context = ctx as RendererContext;
  // For list items, tokens may contain text tokens with nested inline tokens
  // Use parse() for block-level token arrays
  return context.parser?.parse(tokens).trim() ?? fallback;
}

// Custom terminal renderer
const terminalRenderer: RendererObject = {
  // Headings: colored and bold
  heading(token: Tokens.Heading): string {
    const colors = [
      chalk.bold.magenta,  // h1
      chalk.bold.cyan,     // h2
      chalk.bold.green,    // h3
      chalk.bold.yellow,   // h4+
    ];
    const colorFn = colors[Math.min(token.depth - 1, 3)];
    const content = parseInline(this, token.tokens, token.text);
    return '\n' + colorFn(content) + '\n\n';
  },

  // Paragraphs - must parse inline tokens for bold/italic/etc
  paragraph(token: Tokens.Paragraph): string {
    const content = parseInline(this, token.tokens, token.text);
    return content + '\n\n';
  },

  // Bold text
  strong(token: Tokens.Strong): string {
    const content = parseInline(this, token.tokens, token.text);
    return chalk.bold(content);
  },

  // Italic text
  em(token: Tokens.Em): string {
    const content = parseInline(this, token.tokens, token.text);
    return chalk.italic(content);
  },

  // Inline code
  codespan({ text }: Tokens.Codespan): string {
    return chalk.yellow('`' + text + '`');
  },

  // Code blocks - clean format without ``` markers
  code({ text, lang }: Tokens.Code): string {
    const langHeader = lang ? chalk.dim(`  [${lang}]`) + '\n' : '';
    const lines = text.split('\n').map(line => chalk.cyan('    ' + line)).join('\n');
    return '\n' + langHeader + lines + '\n\n';
  },

  // List - parse block tokens for each item (they may contain nested text+inline tokens)
  list(token: Tokens.List): string {
    const result = token.items.map((item, i) => {
      const bullet = token.ordered ? chalk.dim(`${i + 1}.`) : chalk.dim('•');
      const content = parseTokens(this, item.tokens, item.text);
      return `  ${bullet} ${content}`;
    }).join('\n');
    return result + '\n\n';
  },

  // List item - use block parser since items contain text tokens with nested inline
  listitem(token: Tokens.ListItem): string {
    return parseTokens(this, token.tokens, token.text);
  },

  // Links
  link(token: Tokens.Link): string {
    const content = parseInline(this, token.tokens, token.text);
    return chalk.blue.underline(content) + chalk.dim(` (${token.href})`);
  },

  // Blockquotes
  blockquote(token: Tokens.Blockquote): string {
    const text = token.text.trim();
    const lines = text.split('\n').map(line =>
      chalk.dim('│ ') + chalk.italic(line)
    ).join('\n');
    return '\n' + lines + '\n\n';
  },

  // Horizontal rule
  hr(): string {
    return '\n' + chalk.dim('─'.repeat(40)) + '\n\n';
  },

  // Line break
  br(): string {
    return '\n';
  },

  // Delete/strikethrough
  del(token: Tokens.Del): string {
    const content = parseInline(this, token.tokens, token.text);
    return chalk.strikethrough(content);
  },

  // Plain text - may contain nested inline tokens (like in list items)
  text(token: Tokens.Text | Tokens.Escape): string {
    // Tokens.Text can have nested tokens array with inline formatting
    if ('tokens' in token && token.tokens && token.tokens.length > 0) {
      return parseInline(this, token.tokens, token.text);
    }
    return token.text ?? '';
  },

  // HTML (strip tags)
  html(token: Tokens.HTML | Tokens.Tag): string {
    const text = 'text' in token ? token.text : '';
    return text.replace(/<[^>]*>/g, '');
  },
};

// Configure marked with custom renderer
marked.use({ renderer: terminalRenderer });

/**
 * Render markdown text to terminal-formatted string
 */
export function renderMarkdown(text: string): string {
  try {
    const result = marked.parse(text);
    // marked.parse can return string or Promise<string>
    if (typeof result === 'string') {
      return result.trim();
    }
    // If async, return original text (shouldn't happen with sync renderer)
    return text;
  } catch {
    // On error, return original text
    return text;
  }
}
