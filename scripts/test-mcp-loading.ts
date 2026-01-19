#!/usr/bin/env node
/**
 * MCP Loading Test
 *
 * Tests MCP server configuration loading and connection:
 * - Managed servers (Claude Desktop config)
 * - Local servers (.mcp.json files)
 * - Hierarchical loading: managed > local > project > user
 *
 * Usage:
 *   npm run test:mcp                     # Normal mode
 *   GEN_DEBUG_MCP=1 npm run test:mcp     # Debug mode
 *   npm run test:mcp -- --verbose        # Show server details
 */

import { MCPManager } from '../src/mcp/manager.js';
import { logger } from '../src/common/logger.js';

const args = process.argv.slice(2);
const verbose = args.includes('--verbose');

async function main(): Promise<void> {
  console.log('MCP Integration Test\n');

  const cwd = process.cwd();
  const mcpManager = new MCPManager();

  try {
    logger.info('MCPTest', 'Starting MCP loading', { cwd });

    const startTime = Date.now();
    await mcpManager.initialize(cwd);
    const servers = mcpManager.getActiveServers();
    const duration = Date.now() - startTime;

    console.log(`✓ Connected to ${servers.length} MCP servers in ${duration}ms\n`);

    if (servers.length === 0) {
      console.log('No MCP servers configured.\n');
      console.log('To add MCP servers, create .mcp.json in:');
      console.log('  - ~/.mcp.json (user-level)');
      console.log('  - .mcp.json (project-level)\n');
      console.log('Example MCP configuration:');
      console.log(`{
  "mcpServers": {
    "example-server": {
      "command": "npx",
      "args": ["-y", "@example/mcp-server"],
      "env": {}
    }
  }
}\n`);
    } else if (verbose) {
      console.log('Connected MCP servers:');
      for (const server of servers) {
        console.log(`  - ${server.name}`);
        console.log(`    Type: ${server.config.type}`);
        if (server.config.type === 'command') {
          console.log(`    Command: ${server.config.command}`);
          if (server.config.args) {
            console.log(`    Args: ${server.config.args.join(' ')}`);
          }
        }
        console.log(`    Status: ${server.status.status}`);
        if (server.status.status === 'failed' && server.status.error) {
          console.log(`    Error: ${server.status.error}`);
        }
        console.log();
      }
    } else {
      console.log('MCP servers:');
      for (const server of servers) {
        const status = server.status.status === 'connected' ? '✓' : '✗';
        console.log(`  ${status} ${server.name} (${server.status.status})`);
      }
      console.log();
    }

    // Check for failed servers
    const failedServers = servers.filter(s => s.status.status === 'failed');
    if (failedServers.length > 0) {
      console.log(`\n⚠ ${failedServers.length} server(s) failed to connect:`);
      for (const server of failedServers) {
        console.log(`  - ${server.name}: ${server.status.error || 'Unknown error'}`);
      }
      console.log('\nUse GEN_DEBUG_MCP=1 for detailed connection logs.\n');
    }

    // Get tools from MCP servers
    const allTools = mcpManager.getAllTools();
    if (allTools.length > 0) {
      console.log(`MCP tools available: ${allTools.length}`);
      if (verbose) {
        console.log('Tools:');
        for (const tool of allTools.slice(0, 10)) {
          console.log(`  - ${tool.name}: ${tool.description || 'No description'}`);
        }
        if (allTools.length > 10) {
          console.log(`  ... and ${allTools.length - 10} more`);
        }
      }
      console.log();
    }

    process.exit(failedServers.length > 0 ? 1 : 0);
  } catch (error) {
    logger.error('MCPTest', 'MCP loading failed', {
      error: error instanceof Error ? error.message : String(error),
    });
    console.error('✗ Failed to initialize MCP');
    if (error instanceof Error && error.stack) {
      console.error(error.stack);
    }
    process.exit(1);
  }
}

main();
