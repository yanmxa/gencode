# Release Notes - v0.4.0

## 🔄 Breaking Changes

### Provider Rename: Gemini → Google

We've renamed the "Gemini" provider to "Google" for better naming consistency.

**Rationale:**
- Provider = Company (Google, OpenAI, Anthropic)
- Model = Product (Gemini, GPT, Claude)
- Aligns with industry naming patterns

**Changes:**
- Provider ID: `'gemini'` → `'google'`
- TypeScript: `GeminiProvider` → `GoogleProvider`, `GeminiConfig` → `GoogleConfig`

**Unchanged:**
- ✅ Model names: `gemini-2.0-flash`, `gemini-1.5-pro`, etc.
- ✅ Environment variables: `GOOGLE_API_KEY` and `GEMINI_API_KEY` both supported
- ✅ All functionality identical

### Migration Required

**Automatic Migration (Recommended):**

```bash
# macOS
sed -i '' 's/"gemini":/"google":/g' ~/.gen/providers.json
sed -i '' 's/"gemini:/"google:/g' ~/.gen/providers.json
sed -i '' 's/"provider": "gemini"/"provider": "google"/g' ~/.gen/settings.json

# Linux
sed -i 's/"gemini":/"google":/g' ~/.gen/providers.json
sed -i 's/"gemini:/"google:/g' ~/.gen/providers.json
sed -i 's/"provider": "gemini"/"provider": "google"/g' ~/.gen/settings.json
```

**Manual Migration:**

1. **`~/.gen/settings.json`**: Change `"provider": "gemini"` → `"provider": "google"`
2. **`~/.gen/providers.json`**:
   - Rename connection: `"gemini": {...}` → `"google": {...}`
   - Rename model cache: `"gemini:api_key": {...}` → `"google:api_key": {...}`

**For SDK Users:**

```typescript
// Before
import { GeminiProvider, type GeminiConfig } from 'gencode-ai';
const provider = new GeminiProvider();

// After
import { GoogleProvider, type GoogleConfig } from 'gencode-ai';
const provider = new GoogleProvider();
```

---

## ✨ New Features

### Context Management (Proposal 0007)
Manual control over conversation context with new CLI commands:

- **`/compact`** - Trigger conversation compaction manually
  - Summarizes older messages to reduce context
  - Shows before/after statistics

- **`/context`** - View context usage statistics
  - Active vs total messages
  - Compression status
  - Visual progress indicator

### Checkpoint Persistence (Proposal 0008)
Fixed critical persistence bug:

- ✅ Checkpoints saved to disk with sessions
- ✅ Restored when resuming sessions
- ✅ Preserved when forking sessions
- ✅ Session compression works across restarts

### Gemini 3+ Thinking Support
Added support for Gemini 3+ model reasoning:

- Captures `thoughtSignature` from Gemini 3+ models
- Streams reasoning chunks with `type: 'reasoning'`
- Preserves thinking process in tool calls

---

## 🐛 Bug Fixes

- Fixed checkpoint data not persisting across session restarts
- Fixed provider display name for Google in registry
- Improved error messages for unknown provider IDs

---

## 📚 Documentation

- Added session compression implementation guide with flowcharts
- Updated provider documentation for Google rename
- Added migration guide for breaking changes

---

## 🧹 Housekeeping

- Removed temporary test scripts and documentation
- Cleaned up code comments and logging
- Improved checkpoint serialization logic

---

## 📦 Installation

```bash
# New installation
npm install -g gencode-ai@0.4.0

# Update from v0.3.x
npm update -g gencode-ai
```

---

## ⚠️ Important Notes

- **Breaking changes** - Follow migration guide above
- **Backup recommended** - Back up `~/.gen/` before upgrading
- **Config update required** - Use migration script or update manually
- **SDK users** - Update imports from `GeminiProvider` to `GoogleProvider`

---

## 🔗 Links

- [Full Changelog](https://github.com/your-repo/gencode/compare/v0.3.0...v0.4.0)
- [Documentation](https://github.com/your-repo/gencode/docs)
- [Report Issues](https://github.com/your-repo/gencode/issues)
