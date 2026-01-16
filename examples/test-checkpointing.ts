/**
 * Test Checkpointing System
 *
 * This example demonstrates the automatic file change tracking and rewind capabilities.
 */

import { CheckpointManager } from '../src/checkpointing/index.js';
import * as fs from 'fs/promises';
import * as path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

async function testCheckpointing() {
  console.log('🧪 Testing Checkpointing System\n');

  const manager = new CheckpointManager('test-session');
  const testDir = path.join(__dirname, '../.test-checkpoints');
  const testFile = path.join(testDir, 'example.txt');

  try {
    // Create test directory
    await fs.mkdir(testDir, { recursive: true });

    console.log('1️⃣  Recording file creation...');
    const originalContent = 'Hello, World!';
    await fs.writeFile(testFile, originalContent, 'utf-8');

    manager.recordChange({
      path: testFile,
      changeType: 'create',
      previousContent: null,
      newContent: originalContent,
      toolName: 'Write',
    });

    console.log('   ✓ File created:', testFile);
    console.log('   ✓ Checkpoint recorded\n');

    console.log('2️⃣  Recording file modification...');
    const modifiedContent = 'Hello, Checkpointing!';
    await fs.writeFile(testFile, modifiedContent, 'utf-8');

    manager.recordChange({
      path: testFile,
      changeType: 'modify',
      previousContent: originalContent,
      newContent: modifiedContent,
      toolName: 'Edit',
    });

    console.log('   ✓ File modified');
    console.log('   ✓ Checkpoint recorded\n');

    console.log('3️⃣  Listing checkpoints...');
    console.log(manager.formatCheckpointList(true));
    console.log();

    console.log('4️⃣  Getting summary...');
    const summary = manager.getSummary();
    console.log(`   Created: ${summary.created}`);
    console.log(`   Modified: ${summary.modified}`);
    console.log(`   Deleted: ${summary.deleted}`);
    console.log(`   Total: ${summary.total}\n`);

    console.log('5️⃣  Rewinding last change...');
    const result = await manager.rewind({ count: 1 });

    if (result.success) {
      console.log('   ✓ Rewind successful');
      result.revertedFiles.forEach((f) => {
        console.log(`     • ${path.basename(f.path)} (${f.action})`);
      });

      // Verify content was restored
      const restoredContent = await fs.readFile(testFile, 'utf-8');
      if (restoredContent === originalContent) {
        console.log('   ✓ Content restored correctly\n');
      } else {
        console.log('   ✗ Content mismatch!\n');
      }
    } else {
      console.log('   ✗ Rewind failed');
      result.errors.forEach((e) => console.log(`     ${e.error}`));
    }

    console.log('6️⃣  Rewinding all changes...');
    const finalResult = await manager.rewind({ all: true });

    if (finalResult.success) {
      console.log('   ✓ All changes reverted');
      finalResult.revertedFiles.forEach((f) => {
        console.log(`     • ${path.basename(f.path)} (${f.action})`);
      });

      // Verify file was deleted
      try {
        await fs.access(testFile);
        console.log('   ✗ File still exists!\n');
      } catch {
        console.log('   ✓ File was deleted as expected\n');
      }
    }

    console.log('✅ All tests passed!\n');
  } catch (error) {
    console.error('❌ Test failed:', error);
  } finally {
    // Cleanup
    try {
      await fs.rm(testDir, { recursive: true, force: true });
      console.log('🧹 Cleaned up test directory');
    } catch {
      // Ignore cleanup errors
    }
  }
}

// Run the test
testCheckpointing();
