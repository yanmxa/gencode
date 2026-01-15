# Proposal: Auto-updater

- **Proposal ID**: 0035
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement an auto-update mechanism to keep mycode current with the latest features and security fixes, with configurable update policies and rollback support.

## Motivation

Manual updates cause issues:

1. **Version lag**: Users run outdated versions
2. **Security risk**: Missing security patches
3. **Feature gaps**: Delayed access to improvements
4. **Inconsistency**: Teams on different versions
5. **Manual effort**: Extra steps for users

Auto-updates ensure users stay current.

## Detailed Design

### API Design

```typescript
// src/updater/types.ts
interface UpdateConfig {
  enabled: boolean;
  checkInterval: number;      // Hours between checks
  channel: 'stable' | 'beta' | 'nightly';
  autoInstall: boolean;       // Install without asking
  allowDowngrade: boolean;
  rollbackOnError: boolean;
  proxy?: string;
}

interface UpdateInfo {
  version: string;
  releaseDate: Date;
  channel: string;
  changelog: string;
  downloadUrl: string;
  checksum: string;
  size: number;
  breaking?: boolean;
  securityFix?: boolean;
}

interface UpdateResult {
  success: boolean;
  previousVersion: string;
  newVersion: string;
  restartRequired: boolean;
  error?: string;
}
```

### Update Manager

```typescript
// src/updater/manager.ts
class UpdateManager {
  private config: UpdateConfig;
  private currentVersion: string;

  constructor(config?: Partial<UpdateConfig>) {
    this.config = {
      enabled: true,
      checkInterval: 24,
      channel: 'stable',
      autoInstall: false,
      allowDowngrade: false,
      rollbackOnError: true,
      ...config
    };
    this.currentVersion = this.getInstalledVersion();
  }

  async checkForUpdates(): Promise<UpdateInfo | null> {
    const latestUrl = `https://releases.mycode.dev/${this.config.channel}/latest.json`;

    try {
      const response = await fetch(latestUrl);
      const latest = await response.json() as UpdateInfo;

      if (this.isNewerVersion(latest.version)) {
        return latest;
      }
      return null;
    } catch (error) {
      console.error('Update check failed:', error);
      return null;
    }
  }

  async update(info: UpdateInfo): Promise<UpdateResult> {
    const previousVersion = this.currentVersion;

    try {
      // Download update
      const tempPath = await this.download(info);

      // Verify checksum
      if (!await this.verifyChecksum(tempPath, info.checksum)) {
        throw new Error('Checksum verification failed');
      }

      // Backup current installation
      await this.backup();

      // Install update
      await this.install(tempPath);

      // Verify new installation
      if (!await this.verifyInstallation(info.version)) {
        if (this.config.rollbackOnError) {
          await this.rollback();
          throw new Error('Installation verification failed, rolled back');
        }
      }

      return {
        success: true,
        previousVersion,
        newVersion: info.version,
        restartRequired: true
      };
    } catch (error) {
      return {
        success: false,
        previousVersion,
        newVersion: info.version,
        restartRequired: false,
        error: error instanceof Error ? error.message : 'Unknown error'
      };
    }
  }

  async rollback(): Promise<boolean> {
    const backupPath = this.getBackupPath();
    if (!fs.existsSync(backupPath)) {
      return false;
    }

    await this.restore(backupPath);
    return true;
  }

  startBackgroundChecker(): void {
    if (!this.config.enabled) return;

    setInterval(async () => {
      const update = await this.checkForUpdates();
      if (update) {
        if (this.config.autoInstall) {
          await this.update(update);
        } else {
          this.notifyUpdate(update);
        }
      }
    }, this.config.checkInterval * 60 * 60 * 1000);
  }

  private isNewerVersion(version: string): boolean {
    const current = this.parseVersion(this.currentVersion);
    const target = this.parseVersion(version);

    for (let i = 0; i < 3; i++) {
      if (target[i] > current[i]) return true;
      if (target[i] < current[i]) return false;
    }
    return false;
  }

  private parseVersion(version: string): number[] {
    return version.replace(/^v/, '').split('.').map(Number);
  }
}

export const updateManager = new UpdateManager();
```

### CLI Commands

```typescript
// src/cli/commands/update.ts
const updateCommands = {
  '/update check': 'Check for updates',
  '/update install': 'Install available update',
  '/update rollback': 'Rollback to previous version',
  '/update channel <name>': 'Switch update channel',
  '/update auto <on|off>': 'Toggle auto-updates'
};

async function handleUpdate(args: string[]): Promise<void> {
  const [subcommand, ...rest] = args;

  switch (subcommand) {
    case 'check':
      const update = await updateManager.checkForUpdates();
      if (update) {
        console.log(`Update available: ${update.version}`);
        console.log(`Changelog:\n${update.changelog}`);
      } else {
        console.log('You are running the latest version.');
      }
      break;

    case 'install':
      const info = await updateManager.checkForUpdates();
      if (!info) {
        console.log('No update available.');
        return;
      }
      const result = await updateManager.update(info);
      if (result.success) {
        console.log(`Updated to ${result.newVersion}`);
        if (result.restartRequired) {
          console.log('Please restart mycode to complete the update.');
        }
      }
      break;
  }
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/updater/types.ts` | Create | Type definitions |
| `src/updater/manager.ts` | Create | Update logic |
| `src/updater/download.ts` | Create | Download handling |
| `src/updater/verify.ts` | Create | Checksum verification |
| `src/updater/index.ts` | Create | Module exports |
| `src/cli/commands/update.ts` | Create | CLI commands |

## User Experience

### Update Notification
```
┌─ Update Available ────────────────────────────────┐
│                                                   │
│ mycode v2.1.0 is available (you have v2.0.3)     │
│                                                   │
│ Highlights:                                       │
│ • New LSP tool for code intelligence             │
│ • Performance improvements                        │
│ • Security fix for CVE-2025-1234                 │
│                                                   │
│ Run /update install to update                     │
│                                                   │
└───────────────────────────────────────────────────┘
```

### Update Process
```
User: /update install

Downloading mycode v2.1.0 (15.2 MB)...
████████████████████████████████ 100%

Verifying checksum... ✓
Backing up current version... ✓
Installing update... ✓
Verifying installation... ✓

✓ Updated to v2.1.0

Please restart mycode to complete the update.
```

### Rollback
```
User: /update rollback

Rolling back to v2.0.3...

Restoring backup... ✓
Verifying restoration... ✓

✓ Rolled back to v2.0.3

Restart mycode to complete the rollback.
```

## Security Considerations

1. HTTPS for all downloads
2. Checksum verification (SHA-256)
3. Code signing verification
4. Secure backup storage
5. Atomic updates

## Migration Path

1. **Phase 1**: Manual update check
2. **Phase 2**: Download and install
3. **Phase 3**: Background checking
4. **Phase 4**: Auto-install option
5. **Phase 5**: Rollback support

## References

- [Electron Auto-updater](https://www.electron.build/auto-update)
- [Squirrel](https://github.com/Squirrel)
- [Code Signing Best Practices](https://docs.microsoft.com/en-us/windows-hardware/drivers/install/code-signing-best-practices)
