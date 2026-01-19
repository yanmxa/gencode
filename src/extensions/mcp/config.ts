/**
 * MCP Configuration Loader
 * Loads .mcp.json files with hierarchical fallback (Claude Code compatible)
 *
 * Priority (first match wins):
 * 1. Managed (gen)    - /Library/Application Support/GenCode/managed-mcp.json
 * 2. Managed (claude) - /Library/Application Support/ClaudeCode/managed-mcp.json
 * 3. Local (gen)      - ~/.gen.json (under project, mcp section)
 * 4. Local (claude)   - ~/.claude.json (under project, mcp section)
 * 5. Project (gen)    - .gen/.mcp.json
 * 6. Project (claude) - .claude/.mcp.json
 * 7. User (gen)       - ~/.gen/.mcp.json
 * 8. User (claude)    - ~/.claude/.mcp.json
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import type { MCPConfig, MCPServerConfig } from './types.js';
import { expandServerConfig } from './env-expand.js';
import { getManagedPaths as getBaseManagedPaths, findProjectRoot as baseProjectRoot } from '../../base/utils/path-utils.js';

/**
 * Get managed MCP config file paths
 */
function getManagedPaths(): { gen: string; claude: string } {
  const basePaths = getBaseManagedPaths();
  return {
    gen: path.join(basePaths.gen, 'managed-mcp.json'),
    claude: path.join(basePaths.claude, 'managed-mcp.json'),
  };
}

/**
 * Find project root (directory containing .git)
 * Wrapper that returns null instead of cwd when not found
 */
async function findProjectRoot(cwd: string): Promise<string | null> {
  const result = await baseProjectRoot(cwd);
  return result === cwd ? null : result;
}

/**
 * Load MCP config from a file
 */
async function loadMCPFile(filePath: string): Promise<MCPConfig | null> {
  try {
    const content = await fs.readFile(filePath, 'utf-8');
    const config = JSON.parse(content) as MCPConfig;
    return config;
  } catch {
    return null;
  }
}

/**
 * Get config paths in priority order
 */
async function getConfigPaths(cwd: string): Promise<string[]> {
  const paths: string[] = [];
  const projectRoot = await findProjectRoot(cwd);
  const managed = getManagedPaths();
  const homeDir = os.homedir();

  // 1-2. Managed (gen, claude)
  paths.push(managed.gen, managed.claude);

  // 3-4. Local (gen, claude) - under project root if found
  if (projectRoot) {
    paths.push(
      path.join(projectRoot, '.gen.json'),
      path.join(projectRoot, '.claude.json')
    );
  }

  // 5-6. Project (gen, claude)
  if (projectRoot) {
    paths.push(
      path.join(projectRoot, '.gen', '.mcp.json'),
      path.join(projectRoot, '.claude', '.mcp.json')
    );
  }

  // 7-8. User (gen, claude)
  paths.push(
    path.join(homeDir, '.gen', '.mcp.json'),
    path.join(homeDir, '.claude', '.mcp.json')
  );

  return paths;
}

/**
 * Load MCP configuration with fallback
 * Merges all config sources, with earlier sources taking priority
 */
export async function loadMCPConfig(cwd: string = process.cwd()): Promise<MCPConfig> {
  const paths = await getConfigPaths(cwd);
  const servers: Record<string, MCPServerConfig> = {};

  // Load from all paths (reverse order so earlier paths override later)
  for (const filePath of paths.reverse()) {
    const config = await loadMCPFile(filePath);
    if (config?.mcpServers) {
      // Expand environment variables and merge servers
      for (const [name, serverConfig] of Object.entries(config.mcpServers)) {
        // Only add if not already present (earlier paths have priority)
        if (!servers[name]) {
          servers[name] = expandServerConfig(serverConfig as unknown as Record<string, unknown>) as unknown as MCPServerConfig;
        }
      }
    }
  }

  return { mcpServers: servers };
}

/**
 * Save MCP config to user-level file
 */
export async function saveMCPConfig(
  config: MCPConfig,
  scope: 'user' | 'project' = 'user',
  cwd: string = process.cwd()
): Promise<void> {
  const homeDir = os.homedir();
  let filePath: string;

  if (scope === 'user') {
    const dir = path.join(homeDir, '.gen');
    await fs.mkdir(dir, { recursive: true });
    filePath = path.join(dir, '.mcp.json');
  } else {
    // Project scope
    const projectRoot = (await findProjectRoot(cwd)) ?? cwd;
    const dir = path.join(projectRoot, '.gen');
    await fs.mkdir(dir, { recursive: true });
    filePath = path.join(dir, '.mcp.json');
  }

  await fs.writeFile(filePath, JSON.stringify(config, null, 2), 'utf-8');
}

/**
 * Add or update an MCP server in the config
 */
export async function addMCPServer(
  name: string,
  config: MCPServerConfig,
  scope: 'user' | 'project' = 'user',
  cwd: string = process.cwd()
): Promise<void> {
  // Load existing config for the target scope
  const homeDir = os.homedir();
  const filePath =
    scope === 'user'
      ? path.join(homeDir, '.gen', '.mcp.json')
      : path.join((await findProjectRoot(cwd)) ?? cwd, '.gen', '.mcp.json');

  const existing = (await loadMCPFile(filePath)) ?? { mcpServers: {} };

  // Add or update the server
  existing.mcpServers[name] = config;

  // Save back
  await saveMCPConfig(existing, scope, cwd);
}

/**
 * Remove an MCP server from the config
 */
export async function removeMCPServer(
  name: string,
  scope: 'user' | 'project' = 'user',
  cwd: string = process.cwd()
): Promise<void> {
  const homeDir = os.homedir();
  const filePath =
    scope === 'user'
      ? path.join(homeDir, '.gen', '.mcp.json')
      : path.join((await findProjectRoot(cwd)) ?? cwd, '.gen', '.mcp.json');

  const existing = await loadMCPFile(filePath);
  if (!existing || !existing.mcpServers[name]) {
    return; // Server doesn't exist
  }

  // Remove the server
  delete existing.mcpServers[name];

  // Save back
  await saveMCPConfig(existing, scope, cwd);
}
