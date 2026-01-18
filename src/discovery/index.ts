/**
 * Unified Resource Discovery System
 *
 * Public API exports for resource discovery across Commands, Skills, Subagents, and MCP.
 */

// Core types
export type {
  ResourceLevel,
  ResourceNamespace,
  ResourceSource,
  DiscoverableResource,
  FilePattern,
  ResourceParser,
  DiscoveryConfig,
  ResourceDirectory,
} from './types.js';

// Path resolution
export { findProjectRoot, getResourceDirectories, GEN_DIR, CLAUDE_DIR } from './path-resolver.js';

// File scanning
export { scanDirectory, extractResourceName } from './file-scanner.js';

// Resource loading
export { discoverResources, ResourceManager } from './base-loader.js';
