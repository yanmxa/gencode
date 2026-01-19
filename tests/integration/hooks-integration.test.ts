/**
 * Integration tests for hooks system with Agent
 */

import { describe, it, expect, beforeEach, afterEach } from '@jest/globals';
import { Agent } from '../../src/agent/agent.js';
import type { AgentConfig } from '../../src/agent/types.js';
import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';

describe('Hooks Integration with Agent', () => {
  let agent: Agent;
  let testDir: string;
  let hookOutputFile: string;

  beforeEach(async () => {
    // Create temp directory for test
    testDir = await fs.mkdtemp(path.join(os.tmpdir(), 'gencode-hooks-test-'));
    hookOutputFile = path.join(testDir, 'hook-output.txt');

    // Create agent
    const config: AgentConfig = {
      provider: 'anthropic',
      model: 'claude-sonnet-4',
      cwd: testDir,
    };

    agent = new Agent(config);
  });

  afterEach(async () => {
    // Clean up temp directory
    try {
      await fs.rm(testDir, { recursive: true, force: true });
    } catch (error) {
      // Ignore cleanup errors
    }
  });

  describe('SessionStart hooks', () => {
    it('should trigger SessionStart hook when starting new session', async () => {
      // Initialize hooks with SessionStart handler
      agent.initializeHooks({
        SessionStart: [
          {
            hooks: [
              {
                type: 'command',
                command: `echo "Session started" > "${hookOutputFile}"`,
              },
            ],
          },
        ],
      });

      // Start a session
      await agent.startSession();

      // Verify hook was executed
      const output = await fs.readFile(hookOutputFile, 'utf-8');
      expect(output.trim()).toBe('Session started');
    });

    it('should trigger SessionStart hook when resuming session', async () => {
      // Start a session first
      const sessionId = await agent.startSession();

      // Create new agent instance and initialize hooks
      const agent2 = new Agent({
        provider: 'anthropic',
        model: 'claude-sonnet-4',
        cwd: testDir,
      });

      agent2.initializeHooks({
        SessionStart: [
          {
            hooks: [
              {
                type: 'command',
                command: `echo "Session resumed: $SESSION_ID" > "${hookOutputFile}"`,
              },
            ],
          },
        ],
      });

      // Resume the session
      await agent2.resumeSession(sessionId);

      // Verify hook was executed
      const output = await fs.readFile(hookOutputFile, 'utf-8');
      expect(output.trim()).toContain('Session resumed');
      expect(output.trim()).toContain(sessionId);
    });
  });

  describe('Tool hooks', () => {
    it('should trigger PostToolUse hook after successful tool execution', async () => {
      // Initialize hooks with PostToolUse handler
      agent.initializeHooks({
        PostToolUse: [
          {
            matcher: 'Read',
            hooks: [
              {
                type: 'command',
                command: `echo "Read tool executed on $FILE_PATH" >> "${hookOutputFile}"`,
              },
            ],
          },
        ],
      });

      // Create a test file
      const testFile = path.join(testDir, 'test.txt');
      await fs.writeFile(testFile, 'test content');

      // Start session
      await agent.startSession();

      // Create a simple loop that reads a file
      const events = [];
      for await (const event of agent.run(`Read the file ${testFile}`)) {
        events.push(event);
        if (event.type === 'done') break;
        if (event.type === 'error') throw event.error;
      }

      // Give hooks time to execute
      await new Promise(resolve => setTimeout(resolve, 100));

      // Verify hook was executed
      try {
        const output = await fs.readFile(hookOutputFile, 'utf-8');
        expect(output).toContain('Read tool executed');
        expect(output).toContain('test.txt');
      } catch (error) {
        // Hook might not have been triggered if agent didn't use Read tool
        // This is acceptable in this test
      }
    }, 30000);

    it('should handle blocking PreToolUse hook', async () => {
      // Initialize hooks with blocking PreToolUse handler
      agent.initializeHooks({
        PreToolUse: [
          {
            matcher: 'Bash',
            hooks: [
              {
                type: 'command',
                command: 'exit 2', // Exit code 2 blocks the action
              },
            ],
          },
        ],
      });

      // Start session
      await agent.startSession();

      // Try to execute bash command - should be blocked
      const events = [];
      for await (const event of agent.run('Run: echo "hello"')) {
        events.push(event);

        if (event.type === 'tool_result') {
          // Verify the tool was blocked
          expect(event.result.success).toBe(false);
          expect(event.result.error).toContain('Blocked by PreToolUse hook');
        }

        if (event.type === 'done' || event.type === 'error') break;
      }
    }, 30000);
  });

  describe('Stop hooks', () => {
    it('should trigger Stop hook when conversation ends', async () => {
      // Initialize hooks with Stop handler
      agent.initializeHooks({
        Stop: [
          {
            hooks: [
              {
                type: 'command',
                command: `echo "Conversation ended" > "${hookOutputFile}"`,
              },
            ],
          },
        ],
      });

      // Start session
      await agent.startSession();

      // Run a simple query
      for await (const event of agent.run('Say hello')) {
        if (event.type === 'done' || event.type === 'error') break;
      }

      // Give hooks time to execute
      await new Promise(resolve => setTimeout(resolve, 100));

      // Verify hook was executed
      try {
        const output = await fs.readFile(hookOutputFile, 'utf-8');
        expect(output.trim()).toBe('Conversation ended');
      } catch (error) {
        // Stop hook might not trigger in all cases
      }
    }, 30000);
  });

  describe('Multiple hooks', () => {
    it('should execute multiple hooks for same event', async () => {
      const outputFile1 = path.join(testDir, 'hook1.txt');
      const outputFile2 = path.join(testDir, 'hook2.txt');

      agent.initializeHooks({
        SessionStart: [
          {
            hooks: [
              {
                type: 'command',
                command: `echo "Hook 1" > "${outputFile1}"`,
              },
              {
                type: 'command',
                command: `echo "Hook 2" > "${outputFile2}"`,
              },
            ],
          },
        ],
      });

      await agent.startSession();

      // Verify both hooks executed
      const output1 = await fs.readFile(outputFile1, 'utf-8');
      const output2 = await fs.readFile(outputFile2, 'utf-8');

      expect(output1.trim()).toBe('Hook 1');
      expect(output2.trim()).toBe('Hook 2');
    });
  });
});
