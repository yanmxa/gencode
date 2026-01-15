# Proposal: WebFetch Tool

- **Proposal ID**: 0001
- **Author**: Meng Yan
- **Status**: Implemented
- **Created**: 2025-01-15
- **Updated**: 2025-01-15
- **Implemented**: 2025-01-15

## Summary

Implement a WebFetch tool that fetches content from URLs, converts HTML to markdown, and optionally processes the content with an AI model. This enables the agent to access web documentation, API references, and external resources during coding tasks.

## Motivation

Currently, mycode cannot access web content. This limits the agent's ability to:

1. **Read documentation**: Can't fetch API docs or library references
2. **Access examples**: Can't retrieve code examples from GitHub gists
3. **Verify information**: Can't check current versions or changelogs
4. **Gather context**: Can't read linked resources in user messages
5. **Stay current**: Can't access up-to-date information beyond training cutoff

A WebFetch tool enables the agent to access web resources as needed.

## Claude Code Reference

Claude Code's WebFetch tool provides intelligent web content retrieval:

### Tool Definition
```typescript
WebFetch({
  url: "https://example.com/docs",
  prompt: "Extract the API endpoint documentation"
})
```

### Key Features
- Fetches URL content and converts HTML to markdown
- Processes content with AI model using provided prompt
- 15-minute self-cleaning cache for repeated requests
- HTTP auto-upgrade to HTTPS
- Redirect handling with notification
- Large content summarization

### Example Usage
```
User: What's the latest version of React?

Agent: Let me check the React documentation.
[WebFetch: https://react.dev/versions]

Based on the React documentation, the latest version is React 19...
```

### Redirect Handling
When a URL redirects to a different host:
```
The URL redirected to: https://new-host.com/page
Please make a new WebFetch request with this URL.
```

## Detailed Design

### API Design

```typescript
// src/tools/web-fetch/types.ts
interface WebFetchInput {
  url: string;       // URL to fetch (must be valid)
  prompt: string;    // Instructions for processing content
}

interface WebFetchOutput {
  success: boolean;
  content?: string;      // Processed content
  redirectUrl?: string;  // If redirect to different host
  error?: string;
  metadata?: {
    title: string;
    contentLength: number;
    contentType: string;
    fetchedAt: Date;
    cached: boolean;
  };
}

interface WebFetchConfig {
  maxContentSize: number;    // Default: 5MB
  timeout: number;           // Default: 30000ms
  cacheTimeout: number;      // Default: 15 minutes
  userAgent: string;         // Custom user agent
  allowedDomains?: string[]; // Whitelist (optional)
  blockedDomains?: string[]; // Blacklist (optional)
}
```

```typescript
// src/tools/web-fetch/web-fetch-tool.ts
const webFetchTool: Tool<WebFetchInput> = {
  name: 'WebFetch',
  description: `Fetch content from a URL and process it with AI.

Features:
- Converts HTML to clean markdown
- Processes content based on your prompt
- 15-minute cache for repeated requests
- Auto-upgrades HTTP to HTTPS

Usage notes:
- URL must be fully-formed and valid
- Prompt should describe what to extract
- For redirects to different hosts, make a new request with the provided URL
`,
  parameters: z.object({
    url: z.string().url(),
    prompt: z.string().min(1)
  }),
  execute: async (input, context) => { ... }
};
```

### Implementation Approach

1. **URL Validation**: Validate and normalize URLs
2. **Caching**: Implement 15-minute cache with automatic cleanup
3. **HTML to Markdown**: Use turndown or similar library
4. **Content Processing**: Use current provider to process with prompt
5. **Redirect Handling**: Detect cross-host redirects
6. **Size Limits**: Truncate large content with summary

```typescript
// Core fetch logic
async function fetchAndProcess(url: string, prompt: string, context: ToolContext): Promise<WebFetchOutput> {
  // Check cache first
  const cached = cache.get(url);
  if (cached && !cached.expired) {
    return processWithPrompt(cached.content, prompt, context);
  }

  // Fetch URL
  const response = await fetch(url, {
    redirect: 'manual',
    headers: { 'User-Agent': config.userAgent }
  });

  // Handle redirects
  if (response.status >= 300 && response.status < 400) {
    const redirectUrl = response.headers.get('Location');
    if (isExternalRedirect(url, redirectUrl)) {
      return { success: true, redirectUrl };
    }
    return fetchAndProcess(redirectUrl, prompt, context);
  }

  // Convert HTML to markdown
  const html = await response.text();
  const markdown = htmlToMarkdown(html);

  // Cache the content
  cache.set(url, markdown, config.cacheTimeout);

  // Process with AI
  return processWithPrompt(markdown, prompt, context);
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/tools/web-fetch/types.ts` | Create | Type definitions |
| `src/tools/web-fetch/web-fetch-tool.ts` | Create | Tool implementation |
| `src/tools/web-fetch/html-converter.ts` | Create | HTML to markdown conversion |
| `src/tools/web-fetch/cache.ts` | Create | URL content cache |
| `src/tools/web-fetch/index.ts` | Create | Module exports |
| `src/tools/index.ts` | Modify | Register tool |
| `package.json` | Modify | Add turndown dependency |

## User Experience

### Basic Usage
```
User: What does the lodash debounce function do?

Agent: Let me check the lodash documentation.
[WebFetch: https://lodash.com/docs#debounce]

The lodash `debounce` function creates a debounced version of a function
that delays invoking until after `wait` milliseconds have elapsed since
the last time the debounced function was invoked...
```

### Redirect Notification
```
Agent: [WebFetch: https://old-docs.example.com/api]

The URL redirected to a different host: https://new-docs.example.com/api
Let me fetch from the new location.

[WebFetch: https://new-docs.example.com/api]
...
```

### Error Handling
```
Agent: [WebFetch: https://private-server.internal/docs]

Unable to fetch URL: Connection refused
The URL may be inaccessible from my environment.
```

### Cache Indicator
```
Agent: [WebFetch: https://react.dev/docs] (cached)
```

## Alternatives Considered

### Alternative 1: Raw HTML Return
Return raw HTML without processing.

**Pros**: Simpler, faster
**Cons**: Harder for agent to parse, wastes context
**Decision**: Rejected - Markdown conversion is essential

### Alternative 2: Browser Automation
Use Puppeteer/Playwright for JS-rendered sites.

**Pros**: Better for dynamic sites
**Cons**: Heavy dependency, slower, more complex
**Decision**: Deferred - Start with simple fetch, add later if needed

### Alternative 3: No AI Processing
Just return markdown without prompt-based processing.

**Pros**: Simpler, no extra API calls
**Cons**: Large pages waste context, less useful output
**Decision**: Rejected - Processing adds significant value

## Security Considerations

1. **SSRF Prevention**: Block internal/private IPs (localhost, 10.x, 192.168.x, etc.)
2. **Domain Restrictions**: Support allowlist/blocklist configuration
3. **Content Size**: Limit maximum content size
4. **Timeout**: Enforce request timeout
5. **Protocol**: Only allow HTTP/HTTPS
6. **Credentials**: Never send credentials or cookies
7. **User Agent**: Use identifiable user agent string

```typescript
// SSRF protection
const BLOCKED_HOSTS = [
  'localhost', '127.0.0.1', '0.0.0.0',
  /^10\./,
  /^192\.168\./,
  /^172\.(1[6-9]|2[0-9]|3[0-1])\./,
  /^169\.254\./,  // Link-local
  /^::1$/,        // IPv6 localhost
];
```

## Testing Strategy

1. **Unit Tests**:
   - URL validation
   - HTML to markdown conversion
   - Cache behavior
   - SSRF protection

2. **Integration Tests**:
   - End-to-end fetch and process
   - Redirect handling
   - Error cases

3. **Manual Testing**:
   - Various website types
   - Large pages
   - Redirect chains

## Migration Path

1. **Phase 1**: Basic fetch and markdown conversion
2. **Phase 2**: AI processing with prompt
3. **Phase 3**: Caching system
4. **Phase 4**: Domain restrictions and security hardening

No breaking changes to existing functionality.

## Implementation Notes

### Files Created/Modified

| File | Action | Description |
|------|--------|-------------|
| `src/tools/utils/ssrf.ts` | Created | SSRF protection utilities |
| `src/tools/builtin/webfetch.ts` | Created | WebFetch tool implementation |
| `src/tools/types.ts` | Modified | Added ToolResultMetadata interface |
| `src/tools/index.ts` | Modified | Registered webfetchTool |
| `src/cli/components/Messages.tsx` | Modified | Improved tool display (Claude Code style) |
| `src/cli/components/theme.ts` | Modified | Added fetch icon |
| `package.json` | Modified | Added turndown dependency |

### Key Implementation Details

1. **HTML to Markdown**: Uses `turndown` library for conversion
2. **SSRF Protection**: Blocks localhost, private IPs (10.x, 172.16-31.x, 192.168.x), cloud metadata (169.254.169.254)
3. **Size Limits**: 5MB max response size
4. **Timeout**: 30s default, 120s max
5. **Display**: Claude Code style (`● Fetch(url)` with `└ Received 540.3KB (200 OK)`)

### Display Example

```
● Fetch(https://example.com/docs)
  └ Received 540.3KB (200 OK)
```

## References

- [Claude Code WebFetch Documentation](https://code.claude.com/docs/en/tools)
- [Turndown (HTML to Markdown)](https://github.com/mixmark-io/turndown)
- [OWASP SSRF Prevention](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html)
