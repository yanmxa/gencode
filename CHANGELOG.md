# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.5.0] - 2025-01-20

### Changed

- **Major Architecture Refactor**: Reorganized `src/` into a layered modular architecture for better maintainability
- Renamed `extensions/` directory to `ext/` for brevity
- Renamed `infrastructure/` directory to `base/` for simplicity

### Added

- MCP loading improvements with functional tests
- Parent context inheritance for subagents

### Fixed

- Google provider schema conversion now supports `items` and `enum` properties
- `zodToJsonSchema` now correctly calls `schema.toJSONSchema()` instead of `z.toJSONSchema()`
- Test imports updated after architecture reorganization

### Improved

- UI: Compress multi-line ToolCall display to single line with better spacing

## [0.4.1] - Previous Release

- Initial public release with core features
- Multi-provider support (OpenAI, Anthropic, Gemini)
- Tool system with Zod schema validation
- Session management and persistence
- Custom commands and skills support
