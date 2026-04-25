# Changelog

All notable changes to this project will be documented in this file.

## [v1.15.3] - 2026-04-25

### Changed

- Remove thinking-level handling and related model configuration
- Refactor Anthropic and OpenAI client implementations with improved catalog support
- Add catalog tests for Anthropic and OpenAI providers

### Fixed

- Correct Vertex AI integration for Anthropic models

## [v1.15.2] - 2026-04-24

### Changed

- CI: Use current changelog section as release notes instead of full changelog
- Build: Add `release-push` make target to streamline version publishing

### Fixed

- Correct v1.15.1 release notes to show only current version section

## [v1.15.1] - 2026-04-24

### Fixed

- Hide queue badges and queue preview entries for items already sent to inbox
- Keep queue selection focused on the last pending item and exit selection if an item is no longer pending
- Preserve assistant tool-call rendering while tool results are still arriving
- Summarize repeated tool calls in conversation text instead of printing duplicate lines
- Attach `CHANGELOG.md` to GitHub release artifacts

### Tests

- Add coverage for pending queue filtering, hidden queue badge behavior, and aggregated tool-call text output

[Full Changelog](https://github.com/yanmxa/gencode/compare/v1.15.0...v1.15.1)

## [v1.15.0] - 2026-04-24

### Added

- **MiniMax LLM Provider**: Add MiniMax provider implementation with API key, catalog, and client support
- **Cost Tracking**: Add cost calculation for LLM usage with Money and Cost types
- **Conversation Cost Tracking**: Add cost tracking to conversation messages
- **Provider Selection**: Add provider selection and model enrichment

### Fixed

- **API Compatibility**: Fix API compatibility error handling

### Tests

- Add tests for cost estimation and provider selection

[Full Changelog](https://github.com/yanmxa/gencode/compare/v1.14.9...v1.15.0)
