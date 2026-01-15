# Proposal: NotebookEdit Tool

- **Proposal ID**: 0029
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement a NotebookEdit tool for modifying Jupyter notebook cells, enabling the agent to create, edit, and manage interactive notebooks for data science and documentation workflows.

## Motivation

Jupyter notebooks require special handling:

1. **JSON structure**: Can't use regular Edit tool
2. **Cell-based editing**: Need cell-level operations
3. **Output preservation**: Must maintain cell outputs
4. **Metadata handling**: Cell metadata needs preservation
5. **Data science workflows**: Notebooks are essential for ML/data work

NotebookEdit enables proper notebook manipulation.

## Claude Code Reference

Claude Code's NotebookEdit tool provides cell-level operations:

### From Tool Description
```
Completely replaces the contents of a specific cell in a Jupyter notebook.
- notebook_path: absolute path to the .ipynb file
- cell_id: ID of the cell to edit (or position for insert)
- cell_type: 'code' or 'markdown'
- new_source: The new content for the cell
- edit_mode: 'replace', 'insert', or 'delete'
```

### Operations
- **replace**: Replace cell content by ID
- **insert**: Add new cell at position
- **delete**: Remove cell by ID

## Detailed Design

### API Design

```typescript
// src/tools/notebook-edit/types.ts
type CellType = 'code' | 'markdown' | 'raw';
type EditMode = 'replace' | 'insert' | 'delete';

interface NotebookEditInput {
  notebook_path: string;
  cell_id?: string;           // Cell ID for replace/delete
  cell_type?: CellType;       // Required for insert
  new_source: string;         // New cell content
  edit_mode?: EditMode;       // Default: 'replace'
}

interface NotebookEditOutput {
  success: boolean;
  cell_id?: string;           // ID of affected cell
  cell_index?: number;        // Position of affected cell
  error?: string;
}

// Jupyter notebook structure
interface JupyterNotebook {
  nbformat: number;
  nbformat_minor: number;
  metadata: NotebookMetadata;
  cells: JupyterCell[];
}

interface JupyterCell {
  id?: string;                // Cell ID (nbformat 4.5+)
  cell_type: CellType;
  source: string | string[];
  metadata: CellMetadata;
  outputs?: CellOutput[];     // For code cells
  execution_count?: number | null;
}

interface CellMetadata {
  id?: string;
  tags?: string[];
  [key: string]: unknown;
}

interface CellOutput {
  output_type: 'stream' | 'execute_result' | 'display_data' | 'error';
  [key: string]: unknown;
}
```

### NotebookEdit Tool

```typescript
// src/tools/notebook-edit/notebook-edit-tool.ts
const notebookEditTool: Tool<NotebookEditInput> = {
  name: 'NotebookEdit',
  description: `Edit Jupyter notebook cells.

Parameters:
- notebook_path: Absolute path to the .ipynb file
- cell_id: ID of the cell to edit (for replace/delete)
- cell_type: 'code' or 'markdown' (required for insert)
- new_source: New content for the cell
- edit_mode: 'replace' (default), 'insert', or 'delete'

Operations:
- replace: Replace content of existing cell by ID
- insert: Add new cell after specified cell_id (or at start if not specified)
- delete: Remove cell by ID

Notes:
- Cell IDs are preserved when editing
- Cell outputs are cleared when replacing code cells
- Use 0-indexed position or cell_id for targeting
`,
  parameters: z.object({
    notebook_path: z.string(),
    cell_id: z.string().optional(),
    cell_type: z.enum(['code', 'markdown', 'raw']).optional(),
    new_source: z.string(),
    edit_mode: z.enum(['replace', 'insert', 'delete']).optional().default('replace')
  }),
  execute: async (input, context) => {
    return notebookEditor.edit(input, context);
  }
};
```

### Notebook Editor

```typescript
// src/tools/notebook-edit/editor.ts
class NotebookEditor {
  async edit(input: NotebookEditInput, context: ToolContext): Promise<NotebookEditOutput> {
    const fullPath = path.isAbsolute(input.notebook_path)
      ? input.notebook_path
      : path.resolve(context.cwd, input.notebook_path);

    // Read notebook
    if (!fs.existsSync(fullPath)) {
      return { success: false, error: 'Notebook not found' };
    }

    const content = await fs.readFile(fullPath, 'utf-8');
    let notebook: JupyterNotebook;

    try {
      notebook = JSON.parse(content);
    } catch {
      return { success: false, error: 'Invalid notebook format' };
    }

    // Perform operation
    let result: NotebookEditOutput;

    switch (input.edit_mode) {
      case 'replace':
        result = this.replaceCell(notebook, input);
        break;
      case 'insert':
        result = this.insertCell(notebook, input);
        break;
      case 'delete':
        result = this.deleteCell(notebook, input);
        break;
      default:
        return { success: false, error: 'Invalid edit mode' };
    }

    if (!result.success) return result;

    // Write notebook back
    await fs.writeFile(fullPath, JSON.stringify(notebook, null, 1), 'utf-8');

    return result;
  }

  private replaceCell(
    notebook: JupyterNotebook,
    input: NotebookEditInput
  ): NotebookEditOutput {
    const cellIndex = this.findCellIndex(notebook, input.cell_id);

    if (cellIndex === -1) {
      return { success: false, error: `Cell not found: ${input.cell_id}` };
    }

    const cell = notebook.cells[cellIndex];

    // Update source
    cell.source = this.normalizeSource(input.new_source);

    // Update cell type if specified
    if (input.cell_type) {
      cell.cell_type = input.cell_type;
    }

    // Clear outputs for code cells
    if (cell.cell_type === 'code') {
      cell.outputs = [];
      cell.execution_count = null;
    }

    return {
      success: true,
      cell_id: cell.id || input.cell_id,
      cell_index: cellIndex
    };
  }

  private insertCell(
    notebook: JupyterNotebook,
    input: NotebookEditInput
  ): NotebookEditOutput {
    if (!input.cell_type) {
      return { success: false, error: 'cell_type required for insert' };
    }

    const newCell: JupyterCell = {
      id: this.generateCellId(),
      cell_type: input.cell_type,
      source: this.normalizeSource(input.new_source),
      metadata: {}
    };

    if (input.cell_type === 'code') {
      newCell.outputs = [];
      newCell.execution_count = null;
    }

    // Find insertion position
    let insertIndex: number;
    if (input.cell_id) {
      const refIndex = this.findCellIndex(notebook, input.cell_id);
      if (refIndex === -1) {
        return { success: false, error: `Reference cell not found: ${input.cell_id}` };
      }
      insertIndex = refIndex + 1;
    } else {
      insertIndex = 0;  // Insert at beginning
    }

    notebook.cells.splice(insertIndex, 0, newCell);

    return {
      success: true,
      cell_id: newCell.id,
      cell_index: insertIndex
    };
  }

  private deleteCell(
    notebook: JupyterNotebook,
    input: NotebookEditInput
  ): NotebookEditOutput {
    if (!input.cell_id) {
      return { success: false, error: 'cell_id required for delete' };
    }

    const cellIndex = this.findCellIndex(notebook, input.cell_id);

    if (cellIndex === -1) {
      return { success: false, error: `Cell not found: ${input.cell_id}` };
    }

    notebook.cells.splice(cellIndex, 1);

    return {
      success: true,
      cell_id: input.cell_id,
      cell_index: cellIndex
    };
  }

  private findCellIndex(notebook: JupyterNotebook, cellId?: string): number {
    if (!cellId) return -1;

    // Try to find by ID
    const byId = notebook.cells.findIndex(c => c.id === cellId);
    if (byId !== -1) return byId;

    // Try to parse as index
    const index = parseInt(cellId, 10);
    if (!isNaN(index) && index >= 0 && index < notebook.cells.length) {
      return index;
    }

    return -1;
  }

  private normalizeSource(source: string): string[] {
    // Jupyter stores source as array of lines
    return source.split('\n').map((line, i, arr) =>
      i < arr.length - 1 ? line + '\n' : line
    );
  }

  private generateCellId(): string {
    return crypto.randomUUID().replace(/-/g, '').slice(0, 8);
  }
}

export const notebookEditor = new NotebookEditor();
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/tools/notebook-edit/types.ts` | Create | Type definitions |
| `src/tools/notebook-edit/notebook-edit-tool.ts` | Create | Tool implementation |
| `src/tools/notebook-edit/editor.ts` | Create | Notebook editing logic |
| `src/tools/notebook-edit/index.ts` | Create | Module exports |
| `src/tools/index.ts` | Modify | Register tool |

## User Experience

### Replace Cell
```
Agent: [NotebookEdit:
  notebook_path="analysis.ipynb"
  cell_id="abc123"
  new_source="import pandas as pd\nimport numpy as np"
]

✓ Cell updated
  Notebook: analysis.ipynb
  Cell: abc123 (index 0)
  Type: code
```

### Insert Cell
```
Agent: [NotebookEdit:
  notebook_path="analysis.ipynb"
  cell_id="abc123"
  cell_type="markdown"
  new_source="# Data Analysis\n\nThis section analyzes..."
  edit_mode="insert"
]

✓ Cell inserted
  Notebook: analysis.ipynb
  New cell: def456 (index 1)
  Type: markdown
  Inserted after: abc123
```

### Delete Cell
```
Agent: [NotebookEdit:
  notebook_path="analysis.ipynb"
  cell_id="xyz789"
  edit_mode="delete"
]

✓ Cell deleted
  Notebook: analysis.ipynb
  Removed: xyz789 (was at index 5)
```

## Alternatives Considered

### Alternative 1: Full Notebook Rewrite
Use Write tool to replace entire notebook.

**Pros**: Simpler
**Cons**: Loses outputs, risky
**Decision**: Rejected - Need cell-level precision

### Alternative 2: JupyterLab Extension
Integrate with running JupyterLab.

**Pros**: Full notebook capabilities
**Cons**: Requires running server
**Decision**: Deferred - Start with file-based

## Security Considerations

1. **Path Validation**: Validate notebook paths
2. **JSON Validation**: Verify notebook structure
3. **Output Preservation**: Don't execute arbitrary code
4. **Backup Creation**: Optionally backup before edit

## Testing Strategy

1. **Unit Tests**:
   - Cell operations
   - ID generation
   - Source normalization

2. **Integration Tests**:
   - Full edit workflows
   - Multiple operations
   - Edge cases

## Migration Path

1. **Phase 1**: Basic replace/insert/delete
2. **Phase 2**: Batch operations
3. **Phase 3**: Output preservation options
4. **Phase 4**: Notebook creation

## References

- [Jupyter Notebook Format](https://nbformat.readthedocs.io/)
- [nbformat Python Package](https://github.com/jupyter/nbformat)
