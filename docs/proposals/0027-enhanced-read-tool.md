# Proposal: Enhanced Read Tool

- **Proposal ID**: 0027
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Enhance the Read tool with additional capabilities including image reading (multimodal), Jupyter notebook support, line range selection, and smart truncation. This enables the agent to handle diverse file types effectively.

## Motivation

The current Read tool is basic:

1. **Text only**: Can't read images
2. **No notebooks**: Can't handle .ipynb files
3. **Full file loading**: Always reads entire file
4. **Simple truncation**: Just cuts off at limit
5. **No syntax awareness**: Treats all files as plain text

Enhanced reading enables multimodal and structured file access.

## Claude Code Reference

Claude Code's Read tool provides rich file reading:

### From Tool Description
```
- This tool allows Claude Code to read images (PNG, JPG, etc).
  When reading an image file the contents are presented visually.
- This tool can read Jupyter notebooks (.ipynb files) and returns
  all cells with their outputs.
- By default, reads up to 2000 lines from the beginning
- Optionally specify line offset and limit
- Lines longer than 2000 characters will be truncated
- Results returned using cat -n format (line numbers starting at 1)
```

### Key Features
- Multimodal image reading
- Jupyter notebook cell/output display
- Line number prefixes
- Offset and limit parameters
- Smart truncation

## Detailed Design

### API Design

```typescript
// src/tools/read/types.ts
interface ReadInput {
  file_path: string;
  offset?: number;       // Line number to start from (1-based)
  limit?: number;        // Number of lines to read
}

interface ReadOutput {
  success: boolean;
  content?: string;
  contentType: ContentType;
  lineCount: number;
  truncated: boolean;
  metadata?: FileMetadata;
  error?: string;
}

type ContentType =
  | 'text'
  | 'image'
  | 'notebook'
  | 'binary';

interface FileMetadata {
  size: number;
  modified: Date;
  encoding?: string;
  mimeType?: string;
  // For images
  dimensions?: { width: number; height: number };
  // For notebooks
  cellCount?: number;
}

// Notebook types
interface NotebookCell {
  cellType: 'code' | 'markdown' | 'raw';
  source: string;
  outputs?: CellOutput[];
  executionCount?: number;
}

interface NotebookContent {
  cells: NotebookCell[];
  metadata: {
    kernelspec?: { language: string; name: string };
  };
}
```

### Enhanced Read Implementation

```typescript
// src/tools/read/read-tool.ts
const readTool: Tool<ReadInput> = {
  name: 'Read',
  description: `Read file contents from the filesystem.

Parameters:
- file_path: Absolute path to the file
- offset: Line number to start from (1-based, optional)
- limit: Number of lines to read (default: 2000)

Features:
- Reads text files with line numbers
- Displays images visually (PNG, JPG, GIF, etc.)
- Renders Jupyter notebooks with cell outputs
- Smart truncation for large files
- Handles various encodings

Returns content in appropriate format based on file type.
`,
  parameters: z.object({
    file_path: z.string(),
    offset: z.number().int().positive().optional(),
    limit: z.number().int().positive().optional().default(2000)
  }),
  execute: async (input, context) => {
    const fullPath = path.isAbsolute(input.file_path)
      ? input.file_path
      : path.resolve(context.cwd, input.file_path);

    // Check file exists
    if (!fs.existsSync(fullPath)) {
      return { success: false, error: 'File does not exist' };
    }

    const stats = await fs.stat(fullPath);
    const ext = path.extname(fullPath).toLowerCase();

    // Route to appropriate handler
    if (isImageExtension(ext)) {
      return readImage(fullPath, stats);
    }

    if (ext === '.ipynb') {
      return readNotebook(fullPath, stats);
    }

    if (isBinaryExtension(ext)) {
      return { success: false, error: 'Binary files cannot be read as text' };
    }

    return readTextFile(fullPath, input.offset, input.limit, stats);
  }
};
```

### Text File Reading

```typescript
// src/tools/read/readers/text.ts
async function readTextFile(
  filePath: string,
  offset: number = 1,
  limit: number = 2000,
  stats: fs.Stats
): Promise<ReadOutput> {
  const content = await fs.readFile(filePath, 'utf-8');
  const lines = content.split('\n');

  const totalLines = lines.length;
  const startLine = Math.max(1, offset) - 1;  // Convert to 0-indexed
  const endLine = Math.min(startLine + limit, totalLines);

  const selectedLines = lines.slice(startLine, endLine);
  let truncated = false;

  // Format with line numbers (cat -n style)
  const formatted = selectedLines.map((line, idx) => {
    const lineNum = startLine + idx + 1;
    const padding = String(totalLines).length;
    const numStr = String(lineNum).padStart(padding, ' ');

    // Truncate long lines
    let displayLine = line;
    if (line.length > 2000) {
      displayLine = line.slice(0, 2000) + '... (truncated)';
      truncated = true;
    }

    return `${numStr}\t${displayLine}`;
  }).join('\n');

  return {
    success: true,
    content: formatted,
    contentType: 'text',
    lineCount: selectedLines.length,
    truncated: truncated || endLine < totalLines,
    metadata: {
      size: stats.size,
      modified: stats.mtime,
      encoding: 'utf-8'
    }
  };
}
```

### Image Reading

```typescript
// src/tools/read/readers/image.ts
async function readImage(
  filePath: string,
  stats: fs.Stats
): Promise<ReadOutput> {
  const ext = path.extname(filePath).toLowerCase();
  const mimeType = getMimeType(ext);

  // Read as base64 for LLM consumption
  const buffer = await fs.readFile(filePath);
  const base64 = buffer.toString('base64');

  // Get dimensions if possible
  let dimensions: { width: number; height: number } | undefined;
  try {
    dimensions = await getImageDimensions(filePath);
  } catch {
    // Dimensions not available
  }

  return {
    success: true,
    content: `data:${mimeType};base64,${base64}`,
    contentType: 'image',
    lineCount: 0,
    truncated: false,
    metadata: {
      size: stats.size,
      modified: stats.mtime,
      mimeType,
      dimensions
    }
  };
}

function getMimeType(ext: string): string {
  const mimeTypes: Record<string, string> = {
    '.png': 'image/png',
    '.jpg': 'image/jpeg',
    '.jpeg': 'image/jpeg',
    '.gif': 'image/gif',
    '.webp': 'image/webp',
    '.svg': 'image/svg+xml',
    '.bmp': 'image/bmp'
  };
  return mimeTypes[ext] || 'application/octet-stream';
}
```

### Notebook Reading

```typescript
// src/tools/read/readers/notebook.ts
async function readNotebook(
  filePath: string,
  stats: fs.Stats
): Promise<ReadOutput> {
  const content = await fs.readFile(filePath, 'utf-8');
  const notebook = JSON.parse(content) as NotebookContent;

  const formatted = formatNotebook(notebook);

  return {
    success: true,
    content: formatted,
    contentType: 'notebook',
    lineCount: formatted.split('\n').length,
    truncated: false,
    metadata: {
      size: stats.size,
      modified: stats.mtime,
      cellCount: notebook.cells.length
    }
  };
}

function formatNotebook(notebook: NotebookContent): string {
  const parts: string[] = [];

  // Header
  const kernel = notebook.metadata?.kernelspec?.language || 'unknown';
  parts.push(`# Jupyter Notebook (${kernel})`);
  parts.push(`# ${notebook.cells.length} cells\n`);

  // Cells
  notebook.cells.forEach((cell, idx) => {
    parts.push(`## Cell ${idx + 1} [${cell.cellType}]`);

    if (cell.executionCount !== undefined) {
      parts.push(`In [${cell.executionCount}]:`);
    }

    // Source
    const source = Array.isArray(cell.source)
      ? cell.source.join('')
      : cell.source;

    if (cell.cellType === 'code') {
      parts.push('```');
      parts.push(source.trim());
      parts.push('```');
    } else {
      parts.push(source.trim());
    }

    // Outputs
    if (cell.outputs?.length) {
      parts.push('\n**Output:**');
      for (const output of cell.outputs) {
        parts.push(formatCellOutput(output));
      }
    }

    parts.push('');  // Empty line between cells
  });

  return parts.join('\n');
}

function formatCellOutput(output: CellOutput): string {
  switch (output.output_type) {
    case 'stream':
      return output.text?.join('') || '';
    case 'execute_result':
    case 'display_data':
      if (output.data?.['text/plain']) {
        return output.data['text/plain'].join('');
      }
      if (output.data?.['image/png']) {
        return '[Image output]';
      }
      return '[Output data]';
    case 'error':
      return `Error: ${output.ename}: ${output.evalue}`;
    default:
      return '[Unknown output type]';
  }
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/tools/read/types.ts` | Create | Type definitions |
| `src/tools/read/read-tool.ts` | Modify | Enhanced tool |
| `src/tools/read/readers/text.ts` | Create | Text file reader |
| `src/tools/read/readers/image.ts` | Create | Image reader |
| `src/tools/read/readers/notebook.ts` | Create | Notebook reader |
| `src/tools/read/utils.ts` | Create | Shared utilities |
| `src/tools/read/index.ts` | Modify | Updated exports |

## User Experience

### Text File with Line Numbers
```
Agent: [Read: file_path="src/auth.ts"]

     1	import { verify } from 'jsonwebtoken';
     2
     3	export interface AuthConfig {
     4	  secret: string;
     5	  expiresIn: string;
     6	}
     7
     8	export function validateToken(token: string): boolean {
     9	  try {
    10	    verify(token, process.env.JWT_SECRET);
    11	    return true;
    12	  } catch {
    13	    return false;
    14	  }
    15	}
```

### Image Reading
```
Agent: [Read: file_path="docs/architecture.png"]

┌─ Image: architecture.png ─────────────────────────┐
│ [Displayed as image in conversation]             │
│                                                   │
│ Size: 245 KB                                      │
│ Dimensions: 1200 x 800                            │
│ Type: PNG                                         │
└───────────────────────────────────────────────────┘

This architecture diagram shows the three-tier design...
```

### Jupyter Notebook
```
Agent: [Read: file_path="analysis.ipynb"]

# Jupyter Notebook (python)
# 5 cells

## Cell 1 [code]
In [1]:
```
import pandas as pd
import numpy as np
```

## Cell 2 [markdown]
# Data Analysis
This notebook analyzes the sales data...

## Cell 3 [code]
In [2]:
```
df = pd.read_csv('sales.csv')
df.head()
```

**Output:**
   date        product  sales
0  2024-01-01  Widget A   100
1  2024-01-02  Widget B   150
...
```

### Range Reading
```
Agent: [Read: file_path="large-file.ts", offset=100, limit=50]

Reading lines 100-149 of large-file.ts:

   100	  async function processData() {
   101	    const results = await fetch(url);
   ...
   149	  }

(Showing 50 of 1,234 total lines)
```

## Alternatives Considered

### Alternative 1: Separate Image Tool
Create dedicated image reading tool.

**Pros**: Clear separation
**Cons**: More tools to manage
**Decision**: Rejected - Unified Read is cleaner

### Alternative 2: External Libraries
Use sharp/jimp for image processing.

**Pros**: More image capabilities
**Cons**: Heavy dependencies
**Decision**: Deferred - Start with basic reading

### Alternative 3: Always Stream
Stream all file reading.

**Pros**: Memory efficient
**Cons**: Complex for small files
**Decision**: Rejected - Simple read is fine for most

## Security Considerations

1. **Size Limits**: Limit file sizes (e.g., 50MB)
2. **Path Validation**: Prevent path traversal
3. **Binary Detection**: Don't read arbitrary binaries
4. **Memory Management**: Handle large files carefully
5. **Image Validation**: Validate image formats

## Testing Strategy

1. **Unit Tests**:
   - Text file reading
   - Line range selection
   - Image base64 encoding
   - Notebook parsing

2. **Integration Tests**:
   - Various file types
   - Large files
   - Edge cases

3. **Manual Testing**:
   - Real notebooks
   - Various image formats
   - Corrupted files

## Migration Path

1. **Phase 1**: Line numbers and range support
2. **Phase 2**: Image reading
3. **Phase 3**: Notebook support
4. **Phase 4**: Better truncation
5. **Phase 5**: Streaming for large files

Backward compatible with existing Read usage.

## References

- [Jupyter Notebook Format](https://nbformat.readthedocs.io/)
- [Data URLs](https://developer.mozilla.org/en-US/docs/Web/HTTP/Basics_of_HTTP/Data_URLs)
- [File Type Detection](https://github.com/sindresorhus/file-type)
