/**
 * Test script for AskUserQuestion tool
 *
 * Run with: npx tsx examples/test-ask-user.ts [mode]
 *
 * Modes:
 *   single   - Single question, single select (default)
 *   multi    - Single question, multi-select
 *   multiple - Multiple questions with review screen
 */

import { render } from 'ink';
import React from 'react';
import { QuestionPrompt } from '../src/cli/components/QuestionPrompt.js';
import type { Question, QuestionAnswer } from '../src/tools/types.js';

// Test questions - matching Claude Code style
const testQuestions: Question[] = [
  {
    question: 'What type of database would you like to create?',
    header: 'DB Type',
    options: [
      {
        label: 'PostgreSQL',
        description: 'Powerful open-source relational database with advanced features like JSON support and full-text search',
      },
      {
        label: 'MySQL',
        description: 'Popular relational database, great for web applications and general-purpose use',
      },
      {
        label: 'SQLite',
        description: 'Lightweight file-based database, perfect for local development or embedded applications',
      },
      {
        label: 'MongoDB',
        description: 'NoSQL document database for flexible schema and JSON-like documents',
      },
    ],
    multiSelect: false,
  },
];

const testMultiSelectQuestions: Question[] = [
  {
    question: 'Which features should we enable for your project?',
    header: 'Features',
    options: [
      {
        label: 'TypeScript',
        description: 'Type safety and better IDE support with static type checking',
      },
      {
        label: 'ESLint + Prettier',
        description: 'Code linting and automatic formatting for consistent code style',
      },
      {
        label: 'Testing (Vitest)',
        description: 'Fast unit testing framework with native ESM support',
      },
      {
        label: 'Tailwind CSS',
        description: 'Utility-first CSS framework for rapid UI development',
      },
    ],
    multiSelect: true,
  },
];

const testMultipleQuestions: Question[] = [
  {
    question: 'What type of database would you like to create?',
    header: 'DB Type',
    options: [
      {
        label: 'PostgreSQL',
        description: 'Powerful open-source relational database with advanced features like JSON support and full-text search',
      },
      {
        label: 'MySQL',
        description: 'Popular relational database, great for web applications and general-purpose use',
      },
      {
        label: 'SQLite',
        description: 'Lightweight file-based database, perfect for local development or embedded applications',
      },
      {
        label: 'MongoDB',
        description: 'NoSQL document database for flexible schema and JSON-like documents',
      },
    ],
    multiSelect: false,
  },
  {
    question: 'What is the primary purpose of this database?',
    header: 'Purpose',
    options: [
      {
        label: 'GenCode project',
        description: 'Add database functionality to the GenCode AI assistant project',
      },
      {
        label: 'New project',
        description: 'Create a database for a separate new project',
      },
      {
        label: 'Learning/Testing',
        description: 'Set up a database for experimentation and learning',
      },
    ],
    multiSelect: false,
  },
];

// Choose which test to run
const testMode = process.argv[2] || 'single';

let questions: Question[];
switch (testMode) {
  case 'multi':
    questions = testMultiSelectQuestions;
    console.log('\n=== Testing Multi-Select Mode ===\n');
    break;
  case 'multiple':
    questions = testMultipleQuestions;
    console.log('\n=== Testing Multiple Questions with Review ===\n');
    break;
  default:
    questions = testQuestions;
    console.log('\n=== Testing Single-Select Mode ===\n');
}

function TestApp() {
  const handleComplete = (answers: QuestionAnswer[]) => {
    console.log('\n\n=== Answers Received ===');
    answers.forEach((answer, i) => {
      console.log(`\n${i + 1}. ${answer.header}`);
      console.log(`   Question: ${answer.question}`);
      console.log(`   Selected: ${answer.selectedOptions.join(', ') || '(none)'}`);
      if (answer.customInput) {
        console.log(`   Custom: ${answer.customInput}`);
      }
    });
    console.log('\n');
    process.exit(0);
  };

  const handleCancel = () => {
    console.log('\n\nCancelled by user\n');
    process.exit(0);
  };

  return React.createElement(QuestionPrompt, {
    questions,
    onComplete: handleComplete,
    onCancel: handleCancel,
  });
}

// Render the test app
const { unmount } = render(React.createElement(TestApp));

// Handle Ctrl+C
process.on('SIGINT', () => {
  unmount();
  process.exit(0);
});
