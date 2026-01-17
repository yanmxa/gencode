#!/usr/bin/env tsx
/**
 * Migration Script: .gencode → .gen
 *
 * Migrates GenCode configuration from old naming to new:
 * - ~/.gencode/ → ~/.gen/
 * - ./.gencode/ → ./.gen/
 * - AGENT.md → GEN.md
 * - AGENT.local.md → GEN.local.md
 * - providers.json: Old format → New format (provider:authMethod keys)
 *
 * Usage:
 *   npm run migrate          # Run migration
 *   tsx scripts/migrate.ts   # Direct execution
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import * as readline from 'readline';

interface MigrationResult {
  success: boolean;
  migratedPaths: string[];
  errors: string[];
  warnings: string[];
}

/**
 * Check if a path exists
 */
async function pathExists(p: string): Promise<boolean> {
  try {
    await fs.access(p);
    return true;
  } catch {
    return false;
  }
}

/**
 * Rename files within a directory (AGENT.md → GEN.md, AGENT.local.md → GEN.local.md)
 */
async function renameFilesInDir(dir: string): Promise<string[]> {
  const renamed: string[] = [];

  // Rename AGENT.md → GEN.md
  const agentPath = path.join(dir, 'AGENT.md');
  const genPath = path.join(dir, 'GEN.md');

  if (await pathExists(agentPath)) {
    if (await pathExists(genPath)) {
      console.log(`  ⚠️  Skipping ${agentPath} (${genPath} already exists)`);
    } else {
      try {
        await fs.rename(agentPath, genPath);
        renamed.push(`${agentPath} → ${genPath}`);
      } catch (error) {
        throw new Error(`Failed to rename ${agentPath}: ${error instanceof Error ? error.message : String(error)}`);
      }
    }
  }

  // Rename AGENT.local.md → GEN.local.md
  const agentLocalPath = path.join(dir, 'AGENT.local.md');
  const genLocalPath = path.join(dir, 'GEN.local.md');

  if (await pathExists(agentLocalPath)) {
    if (await pathExists(genLocalPath)) {
      console.log(`  ⚠️  Skipping ${agentLocalPath} (${genLocalPath} already exists)`);
    } else {
      try {
        await fs.rename(agentLocalPath, genLocalPath);
        renamed.push(`${agentLocalPath} → ${genLocalPath}`);
      } catch (error) {
        throw new Error(`Failed to rename ${agentLocalPath}: ${error instanceof Error ? error.message : String(error)}`);
      }
    }
  }

  return renamed;
}

/**
 * Migrate providers.json from old format to new format
 * Old format: models[provider] = { provider, authMethod, cachedAt, list }
 * New format: models["provider:authMethod"] = { cachedAt, list }
 */
async function migrateProvidersJson(dryRun = false): Promise<MigrationResult> {
  const result: MigrationResult = {
    success: true,
    migratedPaths: [],
    errors: [],
    warnings: [],
  };

  const providersPath = path.join(os.homedir(), '.gen', 'providers.json');

  // Check if providers.json exists
  if (!(await pathExists(providersPath))) {
    return result; // Nothing to migrate
  }

  try {
    // Read current config
    const content = await fs.readFile(providersPath, 'utf-8');
    const config = JSON.parse(content);

    // Check if migration is needed
    let needsMigration = false;
    const newModels: Record<string, any> = {};

    if (config.models) {
      for (const [key, value] of Object.entries(config.models)) {
        // Detect old format: key doesn't contain ':' AND value has provider/authMethod fields
        if (
          !key.includes(':') &&
          typeof value === 'object' &&
          value !== null &&
          'provider' in value &&
          'authMethod' in value
        ) {
          needsMigration = true;
          const oldCache = value as any;
          const newKey = `${oldCache.provider}:${oldCache.authMethod}`;
          newModels[newKey] = {
            cachedAt: oldCache.cachedAt,
            list: oldCache.list,
          };
        } else {
          // Already new format
          newModels[key] = value;
        }
      }
    }

    if (!needsMigration) {
      return result; // Already in new format
    }

    if (dryRun) {
      result.migratedPaths.push(`[DRY RUN] providers.json: Old format → New format (provider:authMethod keys)`);
      const oldKeys = Object.keys(config.models || {}).filter(k => !k.includes(':'));
      const newKeys = Object.keys(newModels).filter(k => k.includes(':'));
      result.migratedPaths.push(`  Old keys: ${oldKeys.join(', ') || 'none'}`);
      result.migratedPaths.push(`  New keys: ${newKeys.join(', ') || 'none'}`);
    } else {
      // Backup old config
      const backupPath = `${providersPath}.backup`;
      await fs.copyFile(providersPath, backupPath);

      // Update config with new format
      config.models = newModels;

      // Also update connections to ensure authMethod is set
      if (config.connections) {
        for (const [provider, connection] of Object.entries(config.connections)) {
          if (typeof connection === 'object' && connection !== null && !('authMethod' in connection)) {
            // Try to infer authMethod from connection method name
            const method = (connection as any).method;
            if (method?.includes('Vertex')) {
              (connection as any).authMethod = 'vertex';
            } else if (method?.includes('Bedrock')) {
              (connection as any).authMethod = 'bedrock';
            } else if (method?.includes('Azure')) {
              (connection as any).authMethod = 'azure';
            } else {
              (connection as any).authMethod = 'api_key';
            }
          }
        }
      }

      // Write new config
      await fs.writeFile(providersPath, JSON.stringify(config, null, 2));

      result.migratedPaths.push(`providers.json: Migrated to new format`);
      result.migratedPaths.push(`  Backup saved: ${backupPath}`);
    }
  } catch (error) {
    result.errors.push(`Failed to migrate providers.json: ${error instanceof Error ? error.message : String(error)}`);
    result.success = false;
  }

  return result;
}

/**
 * Migrate .gencode to .gen
 */
async function migrateToGen(dryRun = false): Promise<MigrationResult> {
  const result: MigrationResult = {
    success: true,
    migratedPaths: [],
    errors: [],
    warnings: [],
  };

  // 1. Migrate user directory (~/.gencode → ~/.gen)
  const homeGencodePath = path.join(os.homedir(), '.gencode');
  const homeGenPath = path.join(os.homedir(), '.gen');

  if (await pathExists(homeGencodePath)) {
    if (await pathExists(homeGenPath)) {
      result.errors.push(`~/.gen already exists! Please manually merge ~/.gencode into it.`);
      result.success = false;
    } else {
      if (!dryRun) {
        try {
          await fs.rename(homeGencodePath, homeGenPath);
          const renamedFiles = await renameFilesInDir(homeGenPath);
          result.migratedPaths.push(`${homeGencodePath} → ${homeGenPath}`);
          result.migratedPaths.push(...renamedFiles);
        } catch (error) {
          result.errors.push(`Failed to migrate ${homeGencodePath}: ${error instanceof Error ? error.message : String(error)}`);
          result.success = false;
        }
      } else {
        result.migratedPaths.push(`[DRY RUN] ${homeGencodePath} → ${homeGenPath}`);
        result.migratedPaths.push(`[DRY RUN] Will rename AGENT.md → GEN.md in ${homeGenPath}`);
        result.migratedPaths.push(`[DRY RUN] Will rename AGENT.local.md → GEN.local.md in ${homeGenPath}`);
      }
    }
  }

  // 2. Migrate project directory (./.gencode → ./.gen)
  const cwd = process.cwd();
  const projectGencodePath = path.join(cwd, '.gencode');
  const projectGenPath = path.join(cwd, '.gen');

  if (await pathExists(projectGencodePath)) {
    if (await pathExists(projectGenPath)) {
      result.errors.push(`./.gen already exists! Please manually merge ./.gencode into it.`);
      result.success = false;
    } else {
      if (!dryRun) {
        try {
          await fs.rename(projectGencodePath, projectGenPath);
          const renamedFiles = await renameFilesInDir(projectGenPath);
          result.migratedPaths.push(`${projectGencodePath} → ${projectGenPath}`);
          result.migratedPaths.push(...renamedFiles);
        } catch (error) {
          result.errors.push(`Failed to migrate ${projectGencodePath}: ${error instanceof Error ? error.message : String(error)}`);
          result.success = false;
        }
      } else {
        result.migratedPaths.push(`[DRY RUN] ${projectGencodePath} → ${projectGenPath}`);
        result.migratedPaths.push(`[DRY RUN] Will rename AGENT.md → GEN.md in ${projectGenPath}`);
        result.migratedPaths.push(`[DRY RUN] Will rename AGENT.local.md → GEN.local.md in ${projectGenPath}`);
      }
    }
  }

  // 3. Migrate root-level files (./AGENT.md → ./GEN.md)
  const rootAgentPath = path.join(cwd, 'AGENT.md');
  const rootGenPath = path.join(cwd, 'GEN.md');

  if (await pathExists(rootAgentPath)) {
    if (await pathExists(rootGenPath)) {
      result.warnings.push(`./GEN.md already exists! Skipping ./AGENT.md migration.`);
    } else {
      if (!dryRun) {
        try {
          await fs.rename(rootAgentPath, rootGenPath);
          result.migratedPaths.push(`${rootAgentPath} → ${rootGenPath}`);
        } catch (error) {
          result.errors.push(`Failed to migrate ${rootAgentPath}: ${error instanceof Error ? error.message : String(error)}`);
          result.success = false;
        }
      } else {
        result.migratedPaths.push(`[DRY RUN] ${rootAgentPath} → ${rootGenPath}`);
      }
    }
  }

  // 4. Migrate root-level local files (./AGENT.local.md → ./GEN.local.md)
  const rootAgentLocalPath = path.join(cwd, 'AGENT.local.md');
  const rootGenLocalPath = path.join(cwd, 'GEN.local.md');

  if (await pathExists(rootAgentLocalPath)) {
    if (await pathExists(rootGenLocalPath)) {
      result.warnings.push(`./GEN.local.md already exists! Skipping ./AGENT.local.md migration.`);
    } else {
      if (!dryRun) {
        try {
          await fs.rename(rootAgentLocalPath, rootGenLocalPath);
          result.migratedPaths.push(`${rootAgentLocalPath} → ${rootGenLocalPath}`);
        } catch (error) {
          result.errors.push(`Failed to migrate ${rootAgentLocalPath}: ${error instanceof Error ? error.message : String(error)}`);
          result.success = false;
        }
      } else {
        result.migratedPaths.push(`[DRY RUN] ${rootAgentLocalPath} → ${rootGenLocalPath}`);
      }
    }
  }

  // If no migrations needed
  if (result.migratedPaths.length === 0 && result.errors.length === 0) {
    result.warnings.push('No .gencode directories or AGENT.md files found. Already using .gen?');
  }

  return result;
}

/**
 * Check if migration is needed
 */
async function needsMigration(): Promise<boolean> {
  const homeGencodePath = path.join(os.homedir(), '.gencode');
  const projectGencodePath = path.join(process.cwd(), '.gencode');
  const rootAgentPath = path.join(process.cwd(), 'AGENT.md');
  const rootAgentLocalPath = path.join(process.cwd(), 'AGENT.local.md');

  return (
    (await pathExists(homeGencodePath)) ||
    (await pathExists(projectGencodePath)) ||
    (await pathExists(rootAgentPath)) ||
    (await pathExists(rootAgentLocalPath))
  );
}

/**
 * Prompt user for confirmation
 */
async function confirm(message: string): Promise<boolean> {
  const rl = readline.createInterface({
    input: process.stdin,
    output: process.stdout,
  });

  return new Promise((resolve) => {
    rl.question(`${message} (y/N): `, (answer) => {
      rl.close();
      resolve(answer.toLowerCase() === 'y' || answer.toLowerCase() === 'yes');
    });
  });
}

/**
 * Main migration flow
 */
async function main() {
  console.log('🔄 GenCode Migration: .gencode → .gen\n');

  // Check if migration is needed
  const needs = await needsMigration();
  if (!needs) {
    console.log('✅ No .gencode directories or AGENT.md files found.');
    console.log('   Already using .gen? Nothing to migrate.\n');
  }

  // Always check providers.json migration (even if .gencode migration not needed)
  console.log('📋 Scanning for files to migrate...\n');

  // Perform dry run for providers.json
  const providersResult = await migrateProvidersJson(true);

  // Perform dry run for .gencode
  const dryRunResult = await migrateToGen(true);

  // Merge results
  const allMigrations = [...providersResult.migratedPaths, ...dryRunResult.migratedPaths];
  const allErrors = [...providersResult.errors, ...dryRunResult.errors];
  const allWarnings = [...providersResult.warnings, ...dryRunResult.warnings];

  if (allMigrations.length === 0 && allErrors.length === 0) {
    console.log('✅ All configurations are up to date. Nothing to migrate.\n');
    return;
  }

  if (allMigrations.length > 0) {
    console.log('📦 Migration Plan:\n');
    for (const path of allMigrations) {
      console.log(`  ✓ ${path}`);
    }
    console.log('');
  }

  if (allErrors.length > 0) {
    console.log('❌ Errors:\n');
    for (const error of allErrors) {
      console.log(`  ✗ ${error}`);
    }
    console.log('\n⚠️  Migration aborted. Please resolve conflicts manually.\n');
    process.exit(1);
  }

  if (allWarnings.length > 0) {
    console.log('⚠️  Warnings:\n');
    for (const warning of allWarnings) {
      console.log(`  • ${warning}`);
    }
    console.log('');
  }

  // Ask for confirmation
  const confirmed = await confirm('Proceed with migration?');
  if (!confirmed) {
    console.log('\n❌ Migration cancelled.\n');
    return;
  }

  // Execute migration
  console.log('\n🚀 Executing migration...\n');

  // Migrate providers.json first
  const providersExecResult = await migrateProvidersJson(false);

  // Then migrate .gencode
  const result = await migrateToGen(false);

  const allSuccess = providersExecResult.success && result.success;
  const allMigratedPaths = [...providersExecResult.migratedPaths, ...result.migratedPaths];
  const allExecErrors = [...providersExecResult.errors, ...result.errors];

  if (allSuccess) {
    console.log('✅ Migration completed successfully!\n');
    if (allMigratedPaths.length > 0) {
      console.log('Migrated paths:\n');
      for (const path of allMigratedPaths) {
        console.log(`  ✓ ${path}`);
      }
      console.log('');
    }
    console.log('📝 Next steps:\n');
    console.log('  1. Update environment variables: GENCODE_* → GEN_*');
    console.log('     - GENCODE_PROVIDER → GEN_PROVIDER');
    console.log('     - GENCODE_MODEL → GEN_MODEL');
    console.log('     - GENCODE_CONFIG_DIRS → GEN_CONFIG\n');
    console.log('  2. Update any scripts or CI/CD configs\n');
    console.log('  3. Restart GenCode to load from .gen directories\n');
  } else {
    console.log('❌ Migration failed!\n');
    if (allExecErrors.length > 0) {
      console.log('Errors:\n');
      for (const error of allExecErrors) {
        console.log(`  ✗ ${error}`);
      }
      console.log('');
    }
    process.exit(1);
  }
}

main().catch((error) => {
  console.error('❌ Unexpected error:', error);
  process.exit(1);
});
