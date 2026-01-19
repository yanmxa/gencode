/**
 * Base Loader - Unified resource loading with merge strategy
 *
 * Provides a generic resource discovery system that:
 * 1. Scans all configured levels and namespaces
 * 2. Merges resources from all sources
 * 3. Applies priority rules (gen > claude, higher level > lower level)
 */

import type { DiscoverableResource, DiscoveryConfig, ResourceDirectory } from './types.js';
import { getResourceDirectories, findProjectRoot } from './path-resolver.js';
import { scanDirectory, extractResourceName } from './file-scanner.js';
import { logger } from '../common/logger.js';
import { isDebugEnabled } from '../common/debug.js';

/**
 * Discover resources from all configured sources
 *
 * This is the main entry point for resource discovery. It:
 * - Scans all directories in priority order (low to high)
 * - Parses each file using the provided parser
 * - Merges resources into a Map, with higher priority overriding lower
 *
 * @param projectRoot Project root directory (pass process.cwd() if unsure)
 * @param config Discovery configuration
 * @returns Map of resource name to resource
 */
export async function discoverResources<T extends DiscoverableResource>(
  projectRoot: string,
  config: DiscoveryConfig<T>
): Promise<Map<string, T>> {
  // Resolve project root if needed
  const resolvedRoot = await findProjectRoot(projectRoot);

  // Get all resource directories in priority order (low to high)
  const levels = config.levels || ['user', 'project'];
  const directories = await getResourceDirectories(
    resolvedRoot,
    config.subdirectory,
    levels
  );

  // Load resources from all directories
  const resources = new Map<string, T>();

  for (const directory of directories) {
    // Skip non-existent directories
    if (!directory.exists) {
      continue;
    }

    // Scan directory for matching files
    const files = await scanDirectory(directory.path, config.filePattern);

    // Parse each file
    for (const filePath of files) {
      try {
        // Extract resource name
        const name = extractResourceName(
          filePath,
          directory.path,
          config.filePattern
        );

        if (!name) {
          logger.warn(
            config.resourceType,
            `Failed to extract name from file`,
            {
              file: filePath,
              level: directory.level,
              namespace: directory.namespace,
              hint: 'Check file naming convention',
            }
          );
          continue;
        }

        // Validate name
        if (!config.parser.isValidName(name)) {
          logger.warn(
            config.resourceType,
            `Invalid name "${name}"`,
            {
              file: filePath,
              level: directory.level,
              namespace: directory.namespace,
              hint: 'Name must match allowed pattern',
            }
          );
          continue;
        }

        // Parse the file
        const resource = await config.parser.parse(
          filePath,
          directory.level,
          directory.namespace
        );

        if (resource) {
          // Add or override resource (higher priority overwrites lower)
          resources.set(name, resource);

          if (isDebugEnabled('discovery')) {
            logger.debug(
              config.resourceType,
              `Loaded "${name}"`,
              {
                file: filePath,
                level: directory.level,
                namespace: directory.namespace,
              }
            );
          }
        }
      } catch (error) {
        const errorMsg = error instanceof Error ? error.message : String(error);
        logger.warn(
          config.resourceType,
          `Failed to parse file`,
          {
            file: filePath,
            error: errorMsg,
            level: directory.level,
            namespace: directory.namespace,
            hint: 'Check file syntax and required fields',
          }
        );
      }
    }
  }

  return resources;
}

/**
 * Optional Resource Manager class
 *
 * Provides a convenient wrapper around discoverResources with
 * lazy initialization and query methods.
 */
export class ResourceManager<T extends DiscoverableResource> {
  private resources: Map<string, T> = new Map();
  private initialized: boolean = false;

  constructor(private config: DiscoveryConfig<T>) {}

  /**
   * Discover all resources (lazy initialization)
   */
  async discover(projectRoot: string): Promise<void> {
    if (this.initialized) return;

    this.resources = await discoverResources(projectRoot, this.config);
    this.initialized = true;
  }

  /**
   * Get all resources
   */
  getAll(): T[] {
    return Array.from(this.resources.values());
  }

  /**
   * Get a specific resource by name
   */
  get(name: string): T | undefined {
    return this.resources.get(name);
  }

  /**
   * Check if a resource exists
   */
  has(name: string): boolean {
    return this.resources.has(name);
  }

  /**
   * Get the number of resources
   */
  count(): number {
    return this.resources.size;
  }

  /**
   * Get all resource names
   */
  names(): string[] {
    return Array.from(this.resources.keys());
  }

  /**
   * Reload all resources
   */
  async reload(projectRoot: string): Promise<void> {
    this.initialized = false;
    await this.discover(projectRoot);
  }
}
