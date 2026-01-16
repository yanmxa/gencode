/**
 * AskUserQuestion Tool - Structured user questioning
 *
 * Allows the agent to pause execution and present structured questions
 * to the user with predefined options. Supports single-select, multi-select,
 * and custom "Other" input.
 */

import { z } from 'zod';
import type { Tool, ToolResult, ToolContext } from '../types.js';
import { loadToolDescription } from '../../prompts/index.js';

// ============================================================================
// Zod Schemas
// ============================================================================

export const QuestionOptionSchema = z.object({
  label: z
    .string()
    .min(1)
    .max(50)
    .describe('Display text for this option (1-5 words, concise)'),
  description: z
    .string()
    .min(1)
    .max(200)
    .describe('Explanation of what this option means or implications'),
});

export const QuestionSchema = z.object({
  question: z
    .string()
    .min(1)
    .describe('The complete question to ask the user, ending with a question mark'),
  header: z
    .string()
    .min(1)
    .max(12)
    .describe('Very short label displayed as a chip/tag (max 12 chars)'),
  options: z
    .array(QuestionOptionSchema)
    .min(2)
    .max(4)
    .describe('2-4 options for the user to choose from'),
  multiSelect: z
    .boolean()
    .describe('Set to true to allow multiple selections, false for single choice'),
});

export const AskUserQuestionInputSchema = z.object({
  questions: z
    .array(QuestionSchema)
    .min(1)
    .max(4)
    .describe('1-4 questions to ask the user'),
});

// ============================================================================
// Types
// ============================================================================

export type QuestionOption = z.infer<typeof QuestionOptionSchema>;
export type Question = z.infer<typeof QuestionSchema>;
export type AskUserQuestionInput = z.infer<typeof AskUserQuestionInputSchema>;

export interface QuestionAnswer {
  question: string;
  header: string;
  selectedOptions: string[];
  customInput?: string;
}

export interface AskUserQuestionResult {
  answers: QuestionAnswer[];
}

// ============================================================================
// Answer Formatting
// ============================================================================

/**
 * Format answers for display to the agent
 */
export function formatAnswersForAgent(answers: QuestionAnswer[]): string {
  const lines: string[] = ['User answered the following questions:', ''];

  answers.forEach((answer, index) => {
    lines.push(`${index + 1}. ${answer.header} (${answer.question})`);

    if (answer.selectedOptions.length > 0) {
      lines.push(`   Selected: ${answer.selectedOptions.join(', ')}`);
    }

    if (answer.customInput) {
      lines.push(`   Custom input: ${answer.customInput}`);
    }

    lines.push('');
  });

  lines.push('Proceeding with user selections.');

  return lines.join('\n');
}

/**
 * Format answers for CLI confirmation display
 */
export function formatAnswersForDisplay(answers: QuestionAnswer[]): string {
  return answers
    .map((answer) => {
      const selections = answer.customInput
        ? [...answer.selectedOptions, answer.customInput].join(', ')
        : answer.selectedOptions.join(', ');
      return `✔ ${answer.header}: ${selections}`;
    })
    .join('\n');
}

// ============================================================================
// Tool Implementation
// ============================================================================

/**
 * AskUserQuestion tool
 *
 * This tool is special - it doesn't execute immediately but signals
 * the agent loop to pause and wait for user input via the CLI.
 *
 * The actual questioning is handled by the CLI layer (QuestionPrompt component).
 * This tool just validates the input and returns a marker for the agent loop.
 */
export const askUserQuestionTool: Tool<AskUserQuestionInput> = {
  name: 'AskUserQuestion',
  description: loadToolDescription('ask-user'),
  parameters: AskUserQuestionInputSchema,

  async execute(input: AskUserQuestionInput, context: ToolContext): Promise<ToolResult> {
    // Validation is handled by Zod schema
    // Additional validation for recommended options format
    for (const question of input.questions) {
      // Check if first option has (Recommended) - this is just a hint, not enforced
      const firstOption = question.options[0];
      if (firstOption && !firstOption.label.includes('(Recommended)')) {
        // This is fine - recommended suffix is optional
      }
    }

    // Check if context has askUser callback
    if (context.askUser) {
      try {
        const answers = await context.askUser(input.questions);
        return {
          success: true,
          output: formatAnswersForAgent(answers),
          metadata: {
            title: 'AskUserQuestion',
            subtitle: `${answers.length} answer(s) received`,
          },
        };
      } catch (error) {
        return {
          success: false,
          output: '',
          error: `Failed to get user response: ${error instanceof Error ? error.message : String(error)}`,
        };
      }
    }

    // If no askUser callback, return a special marker
    // The agent loop should detect this and handle it specially
    return {
      success: true,
      output: JSON.stringify({
        type: 'ask_user_question',
        questions: input.questions,
        requiresUserInput: true,
      }),
      metadata: {
        title: 'AskUserQuestion',
        subtitle: `${input.questions.length} question(s) pending`,
      },
    };
  },
};
