/**
 * Init Prompt Builder - Generate prompts for /init command
 *
 * Gathers context files (package.json, README.md, etc.) and builds
 * a prompt for the AI to generate AGENT.md
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import { glob } from 'glob';

export interface ContextFiles {
  [path: string]: string;
}

/**
 * Gather context files for project analysis
 */
export async function gatherContextFiles(cwd: string): Promise<ContextFiles> {
  const patterns = [
    'package.json',
    'README.md',
    'CONTRIBUTING.md',
    '.cursor/rules/**/*.md',
    '.cursorrules',
    '.github/copilot-instructions.md',
    'Cargo.toml',
    'go.mod',
    'pyproject.toml',
    'tsconfig.json',
    'Makefile',
    'docker-compose.yml',
    'docker-compose.yaml',
  ];

  const files: ContextFiles = {};

  for (const pattern of patterns) {
    try {
      const matches = await glob(pattern, { cwd, absolute: false });
      for (const match of matches.slice(0, 3)) {
        // Limit per pattern
        try {
          const filePath = path.join(cwd, match);
          const content = await fs.readFile(filePath, 'utf-8');
          if (content.length < 50000) {
            // 50KB limit per file
            files[match] = content;
          }
        } catch {
          // Skip files that can't be read
        }
      }
    } catch {
      // Skip patterns that fail
    }
  }

  return files;
}

/**
 * Build the init prompt for generating AGENT.md
 */
export function buildInitPrompt(contextFiles: ContextFiles, existingAgentMd?: string): string {
  const contextParts = Object.entries(contextFiles)
    .map(([filePath, content]) => `### ${filePath}\n\`\`\`\n${content}\n\`\`\``)
    .join('\n\n');

  const existingSection = existingAgentMd
    ? `\n\n## Existing AGENT.md\n\`\`\`markdown\n${existingAgentMd}\n\`\`\`\n`
    : '';

  return `Please analyze this codebase and create an AGENT.md file, which will be given to future AI assistants to operate in this repository.

## Context Files

${contextParts}
${existingSection}
## Instructions

What to add:
1. Commands that will be commonly used, such as how to build, lint, and run tests.
   Include the necessary commands to develop in this codebase, such as how to run a single test.
2. High-level code architecture and structure so that future instances can be productive more quickly.
   Focus on the "big picture" architecture that requires reading multiple files to understand.

Usage notes:
- ${existingAgentMd ? 'Suggest improvements to the existing AGENT.md.' : 'Create a new AGENT.md.'}
- Do not repeat yourself and do not include obvious instructions like "Provide helpful error messages" or "Write tests for new code".
- Avoid listing every component or file structure that can be easily discovered.
- Don't include generic development practices.
- Do not make up information unless it's expressly included in the files above.
- Keep it concise - aim for ~20-40 lines.
- Start the file with:

# AGENT.md

This file provides guidance to AI assistants when working with code in this repository.

After generating the content, use the Write tool to save it to ./AGENT.md`;
}

/**
 * Get a summary of files found for user feedback
 */
export function getContextSummary(contextFiles: ContextFiles): string {
  const fileNames = Object.keys(contextFiles);
  if (fileNames.length === 0) {
    return 'No context files found';
  }
  return `Found ${fileNames.length} context file(s): ${fileNames.join(', ')}`;
}
