/**
 * Unit tests for checkpoint persistence across session restarts
 */
import { describe, it, expect, beforeEach, afterEach } from '@jest/globals';
import { SessionManager } from '../src/session/manager.js';
import { initCheckpointManager, getCheckpointManager } from '../src/checkpointing/index.js';
import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';

describe('Checkpoint Persistence', () => {
  let sessionManager: SessionManager;
  let testDir: string;

  beforeEach(async () => {
    testDir = path.join(os.tmpdir(), `gencode-test-${Date.now()}`);
    sessionManager = new SessionManager({
      storageDir: testDir,
      maxSessions: 50,
      maxAge: 30,
      autoSave: true,
    });
    await sessionManager.init();
  });

  afterEach(async () => {
    try {
      await fs.rm(testDir, { recursive: true, force: true });
    } catch (error) {
      // Ignore cleanup errors
    }
  });

  it('should save checkpoints with session', async () => {
    // Create session
    const session = await sessionManager.create({
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
      title: 'Test Session',
    });

    // Initialize checkpoint manager
    const checkpointMgr = initCheckpointManager(session.metadata.id);

    // Record some changes
    checkpointMgr.recordChange({
      path: '/test/file1.ts',
      changeType: 'create',
      previousContent: null,
      newContent: 'const x = 1;',
      toolName: 'Write',
    });

    checkpointMgr.recordChange({
      path: '/test/file2.ts',
      changeType: 'modify',
      previousContent: 'const y = 1;',
      newContent: 'const y = 2;',
      toolName: 'Edit',
    });

    // Save session
    await sessionManager.save(session);

    // Read session file
    const filePath = path.join(testDir, `${session.metadata.id}.json`);
    const content = await fs.readFile(filePath, 'utf-8');
    const savedSession = JSON.parse(content);

    // Verify checkpoints were saved
    expect(savedSession.checkpoints).toBeDefined();
    expect(savedSession.checkpoints).toHaveLength(2);
    expect(savedSession.checkpoints[0].path).toBe('/test/file1.ts');
    expect(savedSession.checkpoints[1].changeType).toBe('modify');
  });

  it('should restore checkpoints when loading session', async () => {
    // Create and save session with checkpoints
    const session = await sessionManager.create({
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
      title: 'Test Session',
    });

    const checkpointMgr = initCheckpointManager(session.metadata.id);
    checkpointMgr.recordChange({
      path: '/test/file1.ts',
      changeType: 'create',
      previousContent: null,
      newContent: 'const x = 1;',
      toolName: 'Write',
    });

    await sessionManager.save(session);

    // Load session in new manager instance
    const newSessionManager = new SessionManager({
      storageDir: testDir,
      maxSessions: 50,
      maxAge: 30,
      autoSave: true,
    });
    await newSessionManager.init();

    const loadedSession = await newSessionManager.load(session.metadata.id);
    expect(loadedSession).toBeDefined();

    // Verify checkpoints were restored to manager
    const restoredMgr = getCheckpointManager();
    const checkpoints = restoredMgr.getCheckpoints();

    expect(checkpoints).toHaveLength(1);
    expect(checkpoints[0].path).toBe('/test/file1.ts');
    expect(checkpoints[0].changeType).toBe('create');
  });

  it('should handle session fork with checkpoints', async () => {
    // Create parent session with checkpoints
    const parent = await sessionManager.create({
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
      title: 'Parent Session',
    });

    const parentMgr = initCheckpointManager(parent.metadata.id);
    parentMgr.recordChange({
      path: '/test/file1.ts',
      changeType: 'create',
      previousContent: null,
      newContent: 'const x = 1;',
      toolName: 'Write',
    });

    await sessionManager.save(parent);

    // Fork session
    const forked = await sessionManager.fork(parent.metadata.id, 'Forked Session');
    expect(forked).toBeDefined();
    expect(forked.checkpoints).toHaveLength(1);
    expect((forked.checkpoints as any)[0].path).toBe('/test/file1.ts');
  });

  it('should handle sessions without checkpoints', async () => {
    // Create session without checkpoints
    const session = await sessionManager.create({
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
      title: 'Test Session',
    });

    // Don't add any checkpoints, just save
    await sessionManager.save(session);

    // Load session
    const loaded = await sessionManager.load(session.metadata.id);
    expect(loaded).toBeDefined();
    expect(loaded?.checkpoints).toBeUndefined();
  });

  it('should preserve checkpoint metadata on save/load cycle', async () => {
    // Create session with checkpoints
    const session = await sessionManager.create({
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
      title: 'Test Session',
    });

    const checkpointMgr = initCheckpointManager(session.metadata.id);

    // Record a change with specific metadata
    const now = new Date();
    checkpointMgr.recordChange({
      path: '/test/complex.ts',
      changeType: 'modify',
      previousContent: 'old content',
      newContent: 'new content',
      toolName: 'Edit',
    });

    await sessionManager.save(session);

    // Load session
    const loaded = await sessionManager.load(session.metadata.id);
    expect(loaded).toBeDefined();

    // Verify checkpoint metadata preserved
    const restoredMgr = getCheckpointManager();
    const checkpoints = restoredMgr.getCheckpoints();

    expect(checkpoints).toHaveLength(1);
    expect(checkpoints[0].path).toBe('/test/complex.ts');
    expect(checkpoints[0].changeType).toBe('modify');
    expect(checkpoints[0].previousContent).toBe('old content');
    expect(checkpoints[0].newContent).toBe('new content');
    expect(checkpoints[0].toolName).toBe('Edit');
    expect(checkpoints[0].timestamp).toBeInstanceOf(Date);
  });
});
