/**
 * TaskManager - Orchestrates background subagent execution
 *
 * Responsibilities:
 * - Create and track background tasks
 * - Execute subagents asynchronously
 * - Provide status queries and result retrieval
 * - Handle task cancellation
 * - Clean up old tasks
 */

import * as fs from 'node:fs/promises';
import * as path from 'node:path';
import { homedir } from 'node:os';
import { spawn } from 'node:child_process';
import type { Subagent } from '../../../extensions/subagents/subagent.js';
import type { TaskOutput } from '../../../extensions/subagents/types.js';
import type {
  BackgroundTask,
  BackgroundTaskJson,
  TaskStatus,
  TaskListFilter,
  TaskRegistry,
} from './types.js';
import { OutputStreamer } from './output-streamer.js';

/**
 * Maximum concurrent background tasks
 */
const MAX_CONCURRENT_TASKS = 10;

/**
 * Default task retention period (24 hours)
 */
const DEFAULT_RETENTION_MS = 24 * 60 * 60 * 1000;

/**
 * TaskManager - Manages lifecycle of background tasks
 */
export class TaskManager {
  private tasksDir: string;
  private registryPath: string;
  private runningTasks: Map<string, { promise: Promise<TaskOutput>; abortController: AbortController }>;
  private registry: Map<string, BackgroundTask>;
  private initialized: boolean = false;

  constructor(baseDir?: string) {
    const genDir = baseDir ?? path.join(homedir(), '.gen');
    this.tasksDir = path.join(genDir, 'tasks');
    this.registryPath = path.join(this.tasksDir, 'registry.json');
    this.runningTasks = new Map();
    this.registry = new Map();
  }

  /**
   * Initialize task manager (create directories, load registry)
   */
  private async initialize(): Promise<void> {
    if (this.initialized) return;

    // Ensure tasks directory exists
    await fs.mkdir(this.tasksDir, { recursive: true });

    // Load existing registry
    await this.loadRegistry();

    this.initialized = true;
  }

  /**
   * Create a new background task
   */
  async createTask(
    subagent: Subagent,
    description: string,
    prompt: string
  ): Promise<{ taskId: string; outputFile: string }> {
    await this.initialize();

    // Check concurrent task limit
    const runningCount = Array.from(this.registry.values()).filter(
      (t) => t.status === 'running'
    ).length;

    if (runningCount >= MAX_CONCURRENT_TASKS) {
      throw new Error(`Maximum concurrent tasks (${MAX_CONCURRENT_TASKS}) reached`);
    }

    // Generate task ID
    const taskId = this.generateTaskId(subagent.getConfig().type);

    // Create task directory
    const taskDir = path.join(this.tasksDir, taskId);
    await fs.mkdir(taskDir, { recursive: true });

    // Set up file paths
    const outputFile = path.join(taskDir, 'output.log');
    const metadataFile = path.join(taskDir, 'metadata.json');

    // Create task metadata
    const task: BackgroundTask = {
      id: taskId,
      subagentId: subagent.getId(),
      subagentType: subagent.getConfig().type,
      description,
      status: 'pending',
      outputFile,
      metadataFile,
      startedAt: new Date(),
      model: subagent.getConfig().defaultModel,
    };

    // Save initial metadata
    await this.saveMetadata(task);

    // Add to registry
    this.registry.set(taskId, task);
    await this.updateRegistry();

    // Start async execution (don't await)
    this.executeAsync(taskId, subagent, prompt);

    return { taskId, outputFile };
  }

  /**
   * Create a new background bash task
   */
  async createBashTask(
    command: string,
    description: string,
    cwd: string = process.cwd()
  ): Promise<{ taskId: string; outputFile: string }> {
    await this.initialize();

    // Check concurrent task limit
    const runningCount = Array.from(this.registry.values()).filter(
      (t) => t.status === 'running'
    ).length;

    if (runningCount >= MAX_CONCURRENT_TASKS) {
      throw new Error(`Maximum concurrent tasks (${MAX_CONCURRENT_TASKS}) reached`);
    }

    // Generate task ID
    const taskId = this.generateTaskId('Bash');

    // Create task directory
    const taskDir = path.join(this.tasksDir, taskId);
    await fs.mkdir(taskDir, { recursive: true });

    // Set up file paths
    const outputFile = path.join(taskDir, 'output.log');
    const metadataFile = path.join(taskDir, 'metadata.json');

    // Create task metadata
    const task: BackgroundTask = {
      id: taskId,
      subagentId: taskId, // Use task ID as subagent ID for bash tasks
      subagentType: 'Bash',
      description,
      status: 'pending',
      outputFile,
      metadataFile,
      startedAt: new Date(),
    };

    // Save initial metadata
    await this.saveMetadata(task);

    // Add to registry
    this.registry.set(taskId, task);
    await this.updateRegistry();

    // Start async execution (don't await)
    this.executeBashAsync(taskId, command, cwd);

    return { taskId, outputFile };
  }

  /**
   * Execute bash command in background (fire and forget)
   */
  private async executeBashAsync(
    taskId: string,
    command: string,
    cwd: string
  ): Promise<void> {
    const task = this.registry.get(taskId);
    if (!task) return;

    // Create abort controller for cancellation
    const abortController = new AbortController();

    // Update status to running
    task.status = 'running';
    await this.saveMetadata(task);
    await this.updateRegistry();

    // Create promise for execution
    const executionPromise = (async (): Promise<TaskOutput> => {
      try {
        // Spawn bash process
        const proc = spawn('bash', ['-c', command], {
          cwd,
          env: process.env,
          stdio: ['ignore', 'pipe', 'pipe'],
          detached: false, // Keep attached to properly track completion
        });

        // Open output file for writing
        const outputStream = await fs.open(task.outputFile, 'w');

        // Stream stdout to file
        proc.stdout.on('data', async (data) => {
          const chunk = data.toString();
          await outputStream.write(chunk);
        });

        // Stream stderr to file
        proc.stderr.on('data', async (data) => {
          const chunk = data.toString();
          await outputStream.write(`[stderr] ${chunk}`);
        });

        // Handle abort signal
        abortController.signal.addEventListener('abort', () => {
          proc.kill('SIGTERM');
        });

        // Wait for process to complete
        const exitCode = await new Promise<number>((resolve) => {
          proc.on('close', (code) => {
            resolve(code ?? 1);
          });

          proc.on('error', () => {
            resolve(1);
          });
        });

        // Close output file
        await outputStream.close();

        // Read final output
        let output = '';
        try {
          output = await fs.readFile(task.outputFile, 'utf-8');
          // Truncate if too long
          if (output.length > 10000) {
            output = output.slice(0, 10000) + '\n... (output truncated)';
          }
        } catch {
          output = '(no output)';
        }

        // Update task as completed or error
        if (exitCode === 0) {
          task.status = 'completed';
          task.result = output;
        } else {
          task.status = 'error';
          task.error = `Command exited with code ${exitCode}`;
          task.result = output;
        }

        task.completedAt = new Date();
        task.durationMs = task.completedAt.getTime() - task.startedAt.getTime();

        await this.saveMetadata(task);
        await this.updateRegistry();

        return {
          success: exitCode === 0,
          agentId: taskId,
          result: output,
          error: exitCode !== 0 ? `Exit code ${exitCode}` : undefined,
        };
      } catch (error) {
        // Handle errors
        const errorMessage = error instanceof Error ? error.message : String(error);

        task.status = abortController.signal.aborted ? 'cancelled' : 'error';
        task.completedAt = new Date();
        task.error = errorMessage;
        task.durationMs = task.completedAt.getTime() - task.startedAt.getTime();

        await this.saveMetadata(task);
        await this.updateRegistry();

        return {
          success: false,
          agentId: taskId,
          error: errorMessage,
        };
      } finally {
        // Remove from running tasks
        this.runningTasks.delete(taskId);
      }
    })();

    // Store execution promise
    this.runningTasks.set(taskId, {
      promise: executionPromise,
      abortController,
    });
  }

  /**
   * Execute subagent in background (fire and forget)
   */
  private async executeAsync(
    taskId: string,
    subagent: Subagent,
    prompt: string
  ): Promise<void> {
    const task = this.registry.get(taskId);
    if (!task) return;

    // Create abort controller for cancellation
    const abortController = new AbortController();

    // Create output streamer
    const streamer = new OutputStreamer(task.outputFile, task.metadataFile);

    // Update status to running
    task.status = 'running';
    await this.saveMetadata(task);
    await this.updateRegistry();

    // Create promise for execution
    const executionPromise = (async (): Promise<TaskOutput> => {
      try {
        let turns = 0;
        const maxTurns = subagent.getConfig().maxTurns;

        // Run subagent and stream events
        for await (const event of subagent.getAgent().run(prompt, abortController.signal)) {
          // Write event to log
          await streamer.writeEvent(event);

          // Update progress
          if (event.type === 'done') {
            turns++;
            task.progress = { currentTurn: turns, maxTurns };
            await streamer.updateMetadata({
              turns,
              progress: task.progress,
            });
          }
        }

        // Get final result
        const result = await this.generateResult(subagent);

        // Update task as completed
        task.status = 'completed';
        task.completedAt = new Date();
        task.result = result;
        task.durationMs = task.completedAt.getTime() - task.startedAt.getTime();
        task.turns = turns;

        // Get token usage from agent
        const history = subagent.getAgent().getHistory();
        const lastMessage = history[history.length - 1];
        if (
          lastMessage &&
          'usage' in lastMessage &&
          lastMessage.usage &&
          typeof lastMessage.usage === 'object' &&
          'inputTokens' in lastMessage.usage &&
          'outputTokens' in lastMessage.usage &&
          typeof lastMessage.usage.inputTokens === 'number' &&
          typeof lastMessage.usage.outputTokens === 'number'
        ) {
          task.tokenUsage = {
            input: lastMessage.usage.inputTokens,
            output: lastMessage.usage.outputTokens,
          };
        }

        await this.saveMetadata(task);
        await this.updateRegistry();
        await streamer.close();

        return {
          success: true,
          agentId: subagent.getId(),
          result,
          metadata: {
            subagentType: task.subagentType,
            model: task.model!,
            turns: task.turns,
            durationMs: task.durationMs,
            tokenUsage: task.tokenUsage,
          },
        };
      } catch (error) {
        // Handle errors
        const errorMessage = error instanceof Error ? error.message : String(error);

        task.status = abortController.signal.aborted ? 'cancelled' : 'error';
        task.completedAt = new Date();
        task.error = errorMessage;
        task.durationMs = task.completedAt.getTime() - task.startedAt.getTime();

        await this.saveMetadata(task);
        await this.updateRegistry();
        await streamer.close();

        return {
          success: false,
          agentId: subagent.getId(),
          error: errorMessage,
        };
      } finally {
        // Remove from running tasks
        this.runningTasks.delete(taskId);
      }
    })();

    // Store execution promise
    this.runningTasks.set(taskId, {
      promise: executionPromise,
      abortController,
    });
  }

  /**
   * Generate result summary from subagent
   */
  private async generateResult(subagent: Subagent): Promise<string> {
    const history = subagent.getAgent().getHistory();
    const assistantMessages = history
      .filter((m) => m.role === 'assistant')
      .map((m) => {
        if (typeof m.content === 'string') return m.content;
        if (Array.isArray(m.content)) {
          return m.content
            .filter((b) => b.type === 'text')
            .map((b) => (b as { type: 'text'; text: string }).text)
            .join('\n');
        }
        return '';
      })
      .filter((text) => text.length > 0);

    const combined = assistantMessages.join('\n\n');
    return combined.slice(0, 2000); // Limit to 2000 chars
  }

  /**
   * Get task by ID
   */
  getTask(taskId: string): BackgroundTask | undefined {
    return this.registry.get(taskId);
  }

  /**
   * List tasks with optional filter
   */
  listTasks(filter: TaskListFilter = 'all'): BackgroundTask[] {
    const tasks = Array.from(this.registry.values());

    if (filter === 'all') return tasks;

    return tasks.filter((t) => {
      if (filter === 'running') return t.status === 'running' || t.status === 'pending';
      if (filter === 'completed') return t.status === 'completed';
      if (filter === 'error') return t.status === 'error' || t.status === 'cancelled';
      return true;
    });
  }

  /**
   * Wait for task completion with timeout
   */
  async waitForTask(taskId: string, timeoutMs: number = 60000): Promise<BackgroundTask> {
    const running = this.runningTasks.get(taskId);
    if (!running) {
      // Task not running, return current state
      const task = this.registry.get(taskId);
      if (!task) throw new Error(`Task not found: ${taskId}`);
      return task;
    }

    // Wait for completion with timeout
    const timeoutPromise = new Promise<never>((_, reject) => {
      setTimeout(() => reject(new Error(`Task wait timeout (${timeoutMs}ms)`)), timeoutMs);
    });

    try {
      await Promise.race([running.promise, timeoutPromise]);
      const task = this.registry.get(taskId);
      if (!task) throw new Error(`Task not found after completion: ${taskId}`);
      return task;
    } catch (error) {
      throw error;
    }
  }

  /**
   * Cancel a running task
   */
  async cancelTask(taskId: string): Promise<boolean> {
    const running = this.runningTasks.get(taskId);
    if (!running) return false;

    // Abort execution
    running.abortController.abort();

    // Wait a bit for cleanup
    await new Promise((resolve) => setTimeout(resolve, 100));

    return true;
  }

  /**
   * Clean up old tasks
   */
  async cleanup(maxAgeMs: number = DEFAULT_RETENTION_MS): Promise<number> {
    await this.initialize();

    const now = Date.now();
    const tasks = Array.from(this.registry.values());
    let cleaned = 0;

    for (const task of tasks) {
      // Skip running tasks
      if (task.status === 'running' || task.status === 'pending') continue;

      const completedAt = task.completedAt?.getTime() ?? task.startedAt.getTime();
      const age = now - completedAt;

      if (age > maxAgeMs) {
        // Delete task directory
        const taskDir = path.dirname(task.outputFile);
        await fs.rm(taskDir, { recursive: true, force: true });

        // Remove from registry
        this.registry.delete(task.id);
        cleaned++;
      }
    }

    if (cleaned > 0) {
      await this.updateRegistry();
    }

    return cleaned;
  }

  /**
   * Save task metadata to disk
   */
  private async saveMetadata(task: BackgroundTask): Promise<void> {
    const json: BackgroundTaskJson = {
      ...task,
      startedAt: task.startedAt.toISOString(),
      completedAt: task.completedAt?.toISOString(),
    };

    await fs.writeFile(task.metadataFile, JSON.stringify(json, null, 2), 'utf-8');
  }

  /**
   * Load registry from disk
   */
  private async loadRegistry(): Promise<void> {
    try {
      const data = await fs.readFile(this.registryPath, 'utf-8');
      const registry: TaskRegistry = JSON.parse(data);

      // Convert JSON to BackgroundTask
      for (const [id, taskJson] of Object.entries(registry.tasks)) {
        this.registry.set(id, {
          ...taskJson,
          startedAt: new Date(taskJson.startedAt),
          completedAt: taskJson.completedAt ? new Date(taskJson.completedAt) : undefined,
        });
      }
    } catch (error) {
      // Registry doesn't exist yet, start fresh
      this.registry.clear();
    }
  }

  /**
   * Update registry on disk
   */
  private async updateRegistry(): Promise<void> {
    const tasks: Record<string, BackgroundTaskJson> = {};

    for (const [id, task] of this.registry.entries()) {
      tasks[id] = {
        ...task,
        startedAt: task.startedAt.toISOString(),
        completedAt: task.completedAt?.toISOString(),
      };
    }

    const registry: TaskRegistry = {
      tasks,
      lastUpdated: new Date().toISOString(),
    };

    await fs.writeFile(this.registryPath, JSON.stringify(registry, null, 2), 'utf-8');
  }

  /**
   * Generate unique task ID
   */
  private generateTaskId(subagentType: string): string {
    const timestamp = Date.now();
    const random = Math.random().toString(36).slice(2, 8);
    const type = subagentType.toLowerCase();
    return `bg-${type}-${timestamp}-${random}`;
  }
}
