/**
 * Shared test utilities for memory tests
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';

export interface TestProject {
  tempDir: string;
  projectDir: string;
  cleanup: () => Promise<void>;
}

/**
 * Create a test project with temp directory and git marker
 */
export async function createTestProject(prefix = 'gencode-test-'): Promise<TestProject> {
  const tempDir = await fs.mkdtemp(path.join(os.tmpdir(), prefix));
  const projectDir = path.join(tempDir, 'project');

  await fs.mkdir(projectDir, { recursive: true });
  await fs.mkdir(path.join(projectDir, '.git'));

  return {
    tempDir,
    projectDir,
    cleanup: async () => {
      await fs.rm(tempDir, { recursive: true, force: true });
      delete process.env.GENCODE_CONFIG_DIRS;
    },
  };
}

/**
 * Write memory file to a directory
 */
export async function writeMemory(
  projectDir: string,
  namespace: 'claude' | 'gencode',
  content: string,
  options: { local?: boolean; inDir?: boolean } = {}
): Promise<string> {
  const { local = false, inDir = true } = options;
  const filename = namespace === 'claude'
    ? (local ? 'CLAUDE.local.md' : 'CLAUDE.md')
    : (local ? 'AGENT.local.md' : 'AGENT.md');

  let filePath: string;
  if (inDir) {
    const dir = path.join(projectDir, namespace === 'claude' ? '.claude' : '.gencode');
    await fs.mkdir(dir, { recursive: true });
    filePath = path.join(dir, filename);
  } else {
    filePath = path.join(projectDir, filename);
  }

  await fs.writeFile(filePath, content);
  return filePath;
}
