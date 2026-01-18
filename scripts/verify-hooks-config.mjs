#!/usr/bin/env node
/**
 * Verify hooks configuration loading and fallback mechanism
 *
 * Usage:
 *   node scripts/verify-hooks-config.mjs [directory]
 *
 * If no directory provided, uses current working directory.
 */

import { ConfigManager } from '../dist/config/manager.js';
import { fileURLToPath } from 'url';
import path from 'path';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

async function main() {
  const testDir = process.argv[2] || process.cwd();

  console.log('🔍 Verifying Hooks Configuration');
  console.log('=================================');
  console.log(`📁 Directory: ${testDir}\n`);

  try {
    const manager = new ConfigManager({ cwd: testDir });
    const config = await manager.load();
    const settings = config.settings;

    // Show hooks configuration
    console.log('📊 Loaded Hooks Configuration:');
    console.log('------------------------------');
    if (settings.hooks) {
      console.log(JSON.stringify(settings.hooks, null, 2));
    } else {
      console.log('  (No hooks configured)');
    }

    console.log('\n');

    // Show configuration sources
    console.log('📂 Configuration Sources (in priority order):');
    console.log('--------------------------------------------');
    config.sources.forEach(src => {
      const hasHooks = src.settings.hooks ? '✓ has hooks' : '○ no hooks';
      console.log(`  ${src.level}:${src.namespace.padEnd(6)} - ${hasHooks}`);
      console.log(`    ${src.path}`);
    });

    console.log('\n');

    // Analyze hooks by event
    if (settings.hooks) {
      console.log('🎯 Hooks by Event:');
      console.log('------------------');
      for (const [event, matchers] of Object.entries(settings.hooks)) {
        console.log(`  ${event}:`);
        matchers.forEach((matcher, idx) => {
          const pattern = matcher.matcher || '*';
          const hookCount = matcher.hooks.length;
          console.log(`    [${idx + 1}] Pattern: ${pattern} (${hookCount} hook(s))`);
          matcher.hooks.forEach((hook, hidx) => {
            const cmd = hook.command?.substring(0, 50) || '';
            console.log(`        ${hidx + 1}. ${cmd}${cmd.length >= 50 ? '...' : ''}`);
          });
        });
      }
      console.log('');
    }

    // Analyze fallback behavior
    console.log('🔄 Fallback Analysis:');
    console.log('--------------------');

    const genSources = config.sources.filter(s => s.namespace === 'gen');
    const claudeSources = config.sources.filter(s => s.namespace === 'claude');

    const genHasHooks = genSources.some(s => s.settings.hooks);
    const claudeHasHooks = claudeSources.some(s => s.settings.hooks);

    if (genHasHooks && claudeHasHooks) {
      console.log('  ✅ Both .gen and .claude have hooks');
      console.log('  → Hooks are MERGED (arrays concatenated)');
      console.log('  → .gen hooks loaded first, .claude hooks appended');
    } else if (genHasHooks) {
      console.log('  ✅ Only .gen has hooks');
      console.log('  → Using .gen hooks directly');
    } else if (claudeHasHooks) {
      console.log('  ✅ Only .claude has hooks');
      console.log('  → FALLBACK to .claude hooks');
    } else {
      console.log('  ℹ️  Neither .gen nor .claude have hooks');
    }

    console.log('');

    // Summary
    console.log('📋 Summary:');
    console.log('-----------');
    console.log(`  Total sources loaded: ${config.sources.length}`);
    console.log(`  .gen sources: ${genSources.length}`);
    console.log(`  .claude sources: ${claudeSources.length}`);

    if (settings.hooks) {
      const totalEvents = Object.keys(settings.hooks).length;
      let totalHooks = 0;
      for (const matchers of Object.values(settings.hooks)) {
        matchers.forEach(m => totalHooks += m.hooks.length);
      }
      console.log(`  Events with hooks: ${totalEvents}`);
      console.log(`  Total hooks configured: ${totalHooks}`);
    } else {
      console.log(`  Events with hooks: 0`);
      console.log(`  Total hooks configured: 0`);
    }

  } catch (error) {
    console.error('❌ Error loading configuration:', error.message);
    process.exit(1);
  }
}

main();
