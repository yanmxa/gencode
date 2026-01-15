/**
 * Prompt Matcher - Semantic permission matching for Claude Code style prompts
 *
 * Matches semantic descriptions like "run tests" to actual commands like "npm test".
 * Used for ExitPlanMode allowedPrompts feature.
 */

/**
 * Pattern matcher function type
 */
type PatternMatcher = (input: unknown) => boolean;

/**
 * Built-in semantic patterns for common development operations
 */
const BUILTIN_PATTERNS: Map<string, PatternMatcher> = new Map();

/**
 * Command extraction helpers
 */
function getCommand(input: unknown): string {
  if (typeof input === 'string') return input;
  if (input && typeof input === 'object' && 'command' in input) {
    return String((input as { command: unknown }).command);
  }
  return '';
}

function getFilePath(input: unknown): string {
  if (typeof input === 'string') return input;
  if (input && typeof input === 'object') {
    const obj = input as Record<string, unknown>;
    return String(obj.file_path || obj.filePath || obj.path || '');
  }
  return '';
}

function getUrl(input: unknown): string {
  if (typeof input === 'string') return input;
  if (input && typeof input === 'object' && 'url' in input) {
    return String((input as { url: unknown }).url);
  }
  return '';
}

/**
 * Initialize built-in semantic patterns
 */
function initBuiltinPatterns(): void {
  // ============================================================================
  // Testing
  // ============================================================================
  BUILTIN_PATTERNS.set('run tests', (input) => {
    const cmd = getCommand(input).toLowerCase();
    return (
      cmd.startsWith('npm test') ||
      cmd.startsWith('npm run test') ||
      cmd.startsWith('yarn test') ||
      cmd.startsWith('pnpm test') ||
      cmd.startsWith('bun test') ||
      cmd.startsWith('pytest') ||
      cmd.startsWith('python -m pytest') ||
      cmd.startsWith('go test') ||
      cmd.startsWith('jest') ||
      cmd.startsWith('vitest') ||
      cmd.startsWith('mocha') ||
      cmd.startsWith('cargo test') ||
      cmd.startsWith('make test') ||
      cmd.includes('npm run test') ||
      cmd.includes('npx jest') ||
      cmd.includes('npx vitest')
    );
  });

  // ============================================================================
  // Dependencies
  // ============================================================================
  BUILTIN_PATTERNS.set('install dependencies', (input) => {
    const cmd = getCommand(input).toLowerCase();
    return (
      cmd.startsWith('npm install') ||
      cmd.startsWith('npm i') ||
      cmd.startsWith('npm ci') ||
      cmd.startsWith('yarn install') ||
      cmd.startsWith('yarn add') ||
      cmd.startsWith('pnpm install') ||
      cmd.startsWith('pnpm add') ||
      cmd.startsWith('bun install') ||
      cmd.startsWith('bun add') ||
      cmd.startsWith('pip install') ||
      cmd.startsWith('pip3 install') ||
      cmd.startsWith('poetry install') ||
      cmd.startsWith('go mod download') ||
      cmd.startsWith('go get') ||
      cmd.startsWith('cargo build') ||
      cmd.startsWith('cargo fetch') ||
      cmd.startsWith('bundle install') ||
      cmd.startsWith('composer install')
    );
  });

  // ============================================================================
  // Building
  // ============================================================================
  BUILTIN_PATTERNS.set('build the project', (input) => {
    const cmd = getCommand(input).toLowerCase();
    return (
      cmd.startsWith('npm run build') ||
      cmd.startsWith('yarn build') ||
      cmd.startsWith('pnpm build') ||
      cmd.startsWith('bun run build') ||
      cmd.startsWith('make') ||
      cmd.startsWith('go build') ||
      cmd.startsWith('cargo build') ||
      cmd.startsWith('mvn package') ||
      cmd.startsWith('gradle build') ||
      cmd.startsWith('tsc') ||
      cmd.includes('webpack') ||
      cmd.includes('vite build') ||
      cmd.includes('rollup')
    );
  });

  // ============================================================================
  // Git Operations
  // ============================================================================
  BUILTIN_PATTERNS.set('git operations', (input) => {
    const cmd = getCommand(input).toLowerCase();
    return cmd.startsWith('git ');
  });

  BUILTIN_PATTERNS.set('git status', (input) => {
    const cmd = getCommand(input).toLowerCase();
    return cmd.startsWith('git status') || cmd.startsWith('git diff') || cmd.startsWith('git log');
  });

  BUILTIN_PATTERNS.set('git commit', (input) => {
    const cmd = getCommand(input).toLowerCase();
    return (
      cmd.startsWith('git add') ||
      cmd.startsWith('git commit') ||
      cmd.startsWith('git stash')
    );
  });

  BUILTIN_PATTERNS.set('git push', (input) => {
    const cmd = getCommand(input).toLowerCase();
    return cmd.startsWith('git push');
  });

  BUILTIN_PATTERNS.set('git pull', (input) => {
    const cmd = getCommand(input).toLowerCase();
    return cmd.startsWith('git pull') || cmd.startsWith('git fetch');
  });

  // ============================================================================
  // Linting & Formatting
  // ============================================================================
  BUILTIN_PATTERNS.set('lint code', (input) => {
    const cmd = getCommand(input).toLowerCase();
    return (
      cmd.startsWith('npm run lint') ||
      cmd.startsWith('eslint') ||
      cmd.startsWith('npx eslint') ||
      cmd.startsWith('pylint') ||
      cmd.startsWith('flake8') ||
      cmd.startsWith('golint') ||
      cmd.startsWith('cargo clippy')
    );
  });

  BUILTIN_PATTERNS.set('format code', (input) => {
    const cmd = getCommand(input).toLowerCase();
    return (
      cmd.startsWith('npm run format') ||
      cmd.startsWith('prettier') ||
      cmd.startsWith('npx prettier') ||
      cmd.startsWith('black') ||
      cmd.startsWith('gofmt') ||
      cmd.startsWith('cargo fmt')
    );
  });

  // ============================================================================
  // Development Server
  // ============================================================================
  BUILTIN_PATTERNS.set('start dev server', (input) => {
    const cmd = getCommand(input).toLowerCase();
    return (
      cmd.startsWith('npm run dev') ||
      cmd.startsWith('npm start') ||
      cmd.startsWith('yarn dev') ||
      cmd.startsWith('pnpm dev') ||
      cmd.startsWith('bun dev') ||
      cmd.startsWith('python manage.py runserver') ||
      cmd.startsWith('go run') ||
      cmd.startsWith('cargo run')
    );
  });

  // ============================================================================
  // Read Operations (file system)
  // ============================================================================
  BUILTIN_PATTERNS.set('read files', (input) => {
    const cmd = getCommand(input).toLowerCase();
    return (
      cmd.startsWith('cat ') ||
      cmd.startsWith('less ') ||
      cmd.startsWith('head ') ||
      cmd.startsWith('tail ') ||
      cmd.startsWith('ls ') ||
      cmd.startsWith('find ') ||
      cmd.startsWith('grep ')
    );
  });

  // ============================================================================
  // Type Checking
  // ============================================================================
  BUILTIN_PATTERNS.set('type check', (input) => {
    const cmd = getCommand(input).toLowerCase();
    return (
      cmd.startsWith('tsc --noEmit') ||
      cmd.startsWith('npx tsc') ||
      cmd.startsWith('npm run typecheck') ||
      cmd.startsWith('mypy') ||
      cmd.startsWith('pyright')
    );
  });
}

// Initialize patterns on module load
initBuiltinPatterns();

/**
 * Prompt Matcher class
 * Matches semantic permission prompts to actual tool inputs
 */
export class PromptMatcher {
  private customPatterns: Map<string, PatternMatcher> = new Map();

  /**
   * Register a custom pattern
   */
  registerPattern(prompt: string, matcher: PatternMatcher): void {
    this.customPatterns.set(prompt.toLowerCase(), matcher);
  }

  /**
   * Check if input matches a semantic prompt
   */
  matches(prompt: string, input: unknown): boolean {
    const normalizedPrompt = prompt.toLowerCase().trim();

    // Check custom patterns first
    const customMatcher = this.customPatterns.get(normalizedPrompt);
    if (customMatcher) {
      return customMatcher(input);
    }

    // Check built-in patterns
    const builtinMatcher = BUILTIN_PATTERNS.get(normalizedPrompt);
    if (builtinMatcher) {
      return builtinMatcher(input);
    }

    // Fuzzy matching: check if any keywords from prompt appear in input
    return this.fuzzyMatch(normalizedPrompt, input);
  }

  /**
   * Fuzzy keyword-based matching for custom prompts
   */
  private fuzzyMatch(prompt: string, input: unknown): boolean {
    const inputStr = this.inputToString(input).toLowerCase();

    // Extract meaningful keywords (skip common words)
    const stopWords = new Set([
      'the', 'a', 'an', 'to', 'for', 'in', 'on', 'at', 'with',
      'run', 'execute', 'do', 'perform', 'make', 'this', 'that',
    ]);

    const keywords = prompt
      .split(/\s+/)
      .filter((word) => word.length > 2 && !stopWords.has(word));

    // Require at least one keyword match
    return keywords.some((keyword) => inputStr.includes(keyword));
  }

  /**
   * Convert input to searchable string
   */
  private inputToString(input: unknown): string {
    if (typeof input === 'string') return input;
    if (input === null || input === undefined) return '';

    // Extract relevant fields
    const parts: string[] = [];

    if (typeof input === 'object') {
      const obj = input as Record<string, unknown>;

      // Common input fields
      if (obj.command) parts.push(String(obj.command));
      if (obj.file_path) parts.push(String(obj.file_path));
      if (obj.filePath) parts.push(String(obj.filePath));
      if (obj.path) parts.push(String(obj.path));
      if (obj.url) parts.push(String(obj.url));
      if (obj.query) parts.push(String(obj.query));
      if (obj.pattern) parts.push(String(obj.pattern));
      if (obj.content) parts.push(String(obj.content).slice(0, 100));
    }

    return parts.join(' ');
  }

  /**
   * Get list of available built-in patterns
   */
  getBuiltinPatterns(): string[] {
    return Array.from(BUILTIN_PATTERNS.keys());
  }

  /**
   * Get list of custom patterns
   */
  getCustomPatterns(): string[] {
    return Array.from(this.customPatterns.keys());
  }
}

/**
 * Pattern string parser for Claude Code style patterns
 * Parses "Bash(git add:*)" into { tool: "Bash", pattern: "git add:*" }
 */
export function parsePatternString(pattern: string): { tool: string; pattern?: string } | null {
  // Format: Tool(pattern) or just Tool
  const match = pattern.match(/^(\w+)(?:\(([^)]+)\))?$/);
  if (!match) return null;

  return {
    tool: match[1],
    pattern: match[2],
  };
}

/**
 * Check if command contains shell operators (&&, ||, ;, |).
 * Commands with shell operators should NOT be auto-approved for security.
 */
function containsShellOperators(command: string): boolean {
  // Check for shell operators: &&, ||, ;, |
  // Order matters: check || before | to avoid partial matching
  const operatorPattern = /(?:&&|\|\||[;|])/;
  return operatorPattern.test(command);
}

/**
 * Extract the first command from a shell command string (for display/logging).
 */
function extractFirstCommand(command: string): string {
  const operatorPattern = /\s*(?:&&|\|\||[;|])\s*/;
  const match = command.match(operatorPattern);

  if (match && match.index !== undefined) {
    return command.slice(0, match.index).trim();
  }

  return command.trim();
}

/**
 * Extract file path from input for Read/Edit tools
 */
function extractFilePath(input: unknown): string | null {
  if (typeof input === 'string') return input;
  if (input && typeof input === 'object') {
    const obj = input as Record<string, unknown>;
    const path = obj.file_path ?? obj.filePath ?? obj.path;
    if (typeof path === 'string') return path;
  }
  return null;
}

/**
 * Match a path pattern against a file path (gitignore-style)
 * Supports:
 * - ** for any depth
 * - * for single directory level
 * - ~ for home directory
 */
function matchesPathPattern(pattern: string, filePath: string): boolean {
  // Expand ~ to home directory
  let expandedPattern = pattern;
  if (pattern.startsWith('~/')) {
    const home = process.env.HOME ?? process.env.USERPROFILE ?? '';
    expandedPattern = home + pattern.slice(1);
  }

  // Convert glob pattern to regex
  // First escape special regex chars except * and /
  let regexStr = expandedPattern
    .replace(/[.+^${}()|[\]\\]/g, '\\$&');

  // Handle ** (any depth) - must be done before * handling
  regexStr = regexStr.replace(/\*\*/g, '{{DOUBLE_STAR}}');

  // Handle * (single level, no slashes)
  regexStr = regexStr.replace(/\*/g, '[^/]*');

  // Restore ** as .* (any characters including /)
  regexStr = regexStr.replace(/\{\{DOUBLE_STAR\}\}/g, '.*');

  const regex = new RegExp(`^${regexStr}`);
  return regex.test(filePath);
}

/**
 * Check if an input matches a pattern string
 * Pattern format: "git add:*" or "npm install:*"
 *
 * Supports:
 * - "git:*" - prefix matching (: becomes whitespace)
 * - "npm *" - wildcard matching
 * - "* install" - suffix matching
 * - "git * main" - middle wildcard
 * - Shell operator awareness: "safe-cmd:*" won't match "safe-cmd && malicious-cmd"
 * - Path patterns for Read/Edit: "src/**", "~/.zshrc"
 */
export function matchesPatternString(pattern: string, input: unknown, tool?: string): boolean {
  // For file operations, use path matching
  if (tool && ['Read', 'Edit', 'Write', 'Glob'].includes(tool)) {
    const filePath = extractFilePath(input);
    if (filePath) {
      return matchesPathPattern(pattern, filePath);
    }
  }

  // Extract command string from input
  let inputStr: string;
  if (typeof input === 'string') {
    inputStr = input;
  } else if (input && typeof input === 'object') {
    const obj = input as Record<string, unknown>;
    if (obj.command && typeof obj.command === 'string') {
      const command = obj.command;

      // Shell operator awareness: reject commands with operators for security
      // This prevents bypassing permissions with "safe-cmd && malicious-cmd"
      if (containsShellOperators(command)) {
        return false;
      }

      inputStr = command;
    } else {
      inputStr = JSON.stringify(input);
    }
  } else {
    inputStr = JSON.stringify(input);
  }

  // Convert glob pattern to regex
  // : is a separator (e.g., "git add:*" means "git add" followed by anything)
  const regexStr = pattern
    .replace(/[.+^${}()|[\]\\]/g, '\\$&') // Escape special chars except * and :
    .replace(/:/g, '\\s*') // : becomes optional whitespace
    .replace(/\*/g, '.*'); // * becomes .*

  const regex = new RegExp(`^${regexStr}`, 'i');
  return regex.test(inputStr);
}
