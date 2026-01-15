# Proposal: WebSearch Tool

- **Proposal ID**: 0002
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement a WebSearch tool that enables the agent to search the web and use results to inform responses. This provides access to current information, recent documentation, and up-to-date technical resources beyond the model's training cutoff.

## Motivation

Currently, mycode cannot search the web. This limits the agent when:

1. **Outdated information**: Model knowledge has a cutoff date
2. **New libraries**: Can't find docs for recently released packages
3. **Current events**: Can't access recent security advisories, updates
4. **Error debugging**: Can't search for recent solutions to errors
5. **Best practices**: Can't find current community recommendations

A WebSearch tool enables access to current web information.

## Claude Code Reference

Claude Code's WebSearch tool provides intelligent web searching:

### Tool Definition
```typescript
WebSearch({
  query: "React 19 new features 2025",
  allowed_domains: ["react.dev", "github.com"],  // Optional
  blocked_domains: ["spam-site.com"]             // Optional
})
```

### Key Features
- Returns formatted search results with links
- Supports domain filtering (allow/block lists)
- Results include titles and URLs as markdown hyperlinks
- Requires source citation in responses
- Minimum 2-character query

### Example Usage
```
User: What's new in TypeScript 5.4?

Agent: Let me search for the latest TypeScript updates.
[WebSearch: "TypeScript 5.4 new features 2025"]

Based on my search, TypeScript 5.4 introduces:
- NoInfer utility type for better type inference
- Improved narrowing for closures
...

Sources:
- [TypeScript 5.4 Release Notes](https://www.typescriptlang.org/docs/...)
- [What's New in TypeScript 5.4](https://devblogs.microsoft.com/...)
```

### Source Citation Requirement
Claude Code mandates including sources after answering:
```
[Your answer here]

Sources:
- [Source Title 1](https://example.com/1)
- [Source Title 2](https://example.com/2)
```

## Detailed Design

### API Design

```typescript
// src/tools/web-search/types.ts
interface WebSearchInput {
  query: string;              // Search query (min 2 chars)
  allowed_domains?: string[]; // Only include these domains
  blocked_domains?: string[]; // Exclude these domains
}

interface SearchResult {
  title: string;
  url: string;
  snippet: string;
  domain: string;
}

interface WebSearchOutput {
  success: boolean;
  results?: SearchResult[];
  query: string;
  error?: string;
  metadata?: {
    resultCount: number;
    searchTime: number;
  };
}

interface WebSearchConfig {
  provider: 'google' | 'bing' | 'duckduckgo' | 'serper';
  apiKey?: string;
  maxResults: number;          // Default: 10
  timeout: number;             // Default: 10000ms
  safeSearch: boolean;         // Default: true
}
```

```typescript
// src/tools/web-search/web-search-tool.ts
const webSearchTool: Tool<WebSearchInput> = {
  name: 'WebSearch',
  description: `Search the web for current information.

Use this tool when you need:
- Up-to-date information beyond your knowledge cutoff
- Current documentation or release notes
- Recent solutions to technical problems
- Current best practices

IMPORTANT: After answering, include a "Sources:" section with all relevant URLs as markdown hyperlinks.

Example:
  [Your answer]

  Sources:
  - [Title 1](https://url1)
  - [Title 2](https://url2)
`,
  parameters: z.object({
    query: z.string().min(2),
    allowed_domains: z.array(z.string()).optional(),
    blocked_domains: z.array(z.string()).optional()
  }),
  execute: async (input, context) => { ... }
};
```

### Implementation Approach

1. **Search Provider**: Integrate with search API (Serper, Google, Bing, or DuckDuckGo)
2. **Query Processing**: Clean and optimize search queries
3. **Domain Filtering**: Apply allow/block lists to results
4. **Result Formatting**: Format results as markdown with links
5. **Rate Limiting**: Respect API rate limits

```typescript
// Search provider abstraction
interface SearchProvider {
  search(query: string, options: SearchOptions): Promise<SearchResult[]>;
}

// Serper.dev implementation (recommended)
class SerperProvider implements SearchProvider {
  async search(query: string, options: SearchOptions): Promise<SearchResult[]> {
    const response = await fetch('https://google.serper.dev/search', {
      method: 'POST',
      headers: {
        'X-API-KEY': this.apiKey,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        q: query,
        num: options.maxResults
      })
    });

    const data = await response.json();
    return data.organic.map(r => ({
      title: r.title,
      url: r.link,
      snippet: r.snippet,
      domain: new URL(r.link).hostname
    }));
  }
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/tools/web-search/types.ts` | Create | Type definitions |
| `src/tools/web-search/web-search-tool.ts` | Create | Tool implementation |
| `src/tools/web-search/providers/` | Create | Search provider implementations |
| `src/tools/web-search/index.ts` | Create | Module exports |
| `src/tools/index.ts` | Modify | Register tool |
| `src/config/settings-manager.ts` | Modify | Add search config |

## User Experience

### Basic Search
```
User: How do I use the new Bun test runner?

Agent: Let me search for the latest Bun testing documentation.
[WebSearch: "Bun test runner documentation 2025"]

Based on my search, here's how to use Bun's test runner:

1. Create a test file with `.test.ts` extension
2. Import `expect` and `test` from `bun:test`
3. Run with `bun test`

```typescript
import { expect, test } from "bun:test";

test("2 + 2", () => {
  expect(2 + 2).toBe(4);
});
```

Sources:
- [Bun Test Runner](https://bun.sh/docs/cli/test)
- [Bun Testing Guide](https://bun.sh/guides/test/run-tests)
```

### Domain-Filtered Search
```
User: Search for React hooks best practices, only from official sources

Agent: [WebSearch: "React hooks best practices", allowed_domains: ["react.dev", "reactjs.org"]]

Here are the best practices from official React documentation...

Sources:
- [Rules of Hooks](https://react.dev/reference/rules/rules-of-hooks)
```

### Error Case
```
Agent: [WebSearch: "..."]

I wasn't able to perform the web search. This might be due to:
- No search API configured
- Rate limit exceeded
- Network connectivity issues

I'll answer based on my training data instead...
```

## Alternatives Considered

### Alternative 1: Scraping-Based Search
Scrape Google/Bing results directly.

**Pros**: No API costs
**Cons**: Terms of service violations, blocking, reliability
**Decision**: Rejected - Use proper search APIs

### Alternative 2: Single Provider Only
Only support one search provider.

**Pros**: Simpler implementation
**Cons**: Lock-in, single point of failure
**Decision**: Rejected - Provider abstraction adds flexibility

### Alternative 3: No Domain Filtering
Skip allow/block list feature.

**Pros**: Simpler
**Cons**: Less control over result quality
**Decision**: Rejected - Filtering adds significant value

## Security Considerations

1. **API Key Security**: Store search API keys securely
2. **Query Sanitization**: Sanitize search queries
3. **Result Validation**: Validate returned URLs
4. **Rate Limiting**: Prevent abuse of search API
5. **Safe Search**: Enable safe search by default
6. **Content Filtering**: Consider filtering adult/malicious content

## Testing Strategy

1. **Unit Tests**:
   - Query validation
   - Domain filtering logic
   - Result formatting

2. **Integration Tests**:
   - Provider integration (with mock)
   - End-to-end search flow

3. **Manual Testing**:
   - Various query types
   - Domain filtering
   - Error handling

## Migration Path

1. **Phase 1**: Basic search with Serper provider
2. **Phase 2**: Domain filtering
3. **Phase 3**: Additional providers
4. **Phase 4**: Search result caching

Configuration required: User must provide search API key.

## References

- [Claude Code WebSearch Documentation](https://code.claude.com/docs/en/tools)
- [Serper API](https://serper.dev/)
- [Google Custom Search API](https://developers.google.com/custom-search)
- [Bing Web Search API](https://www.microsoft.com/en-us/bing/apis/bing-web-search-api)
