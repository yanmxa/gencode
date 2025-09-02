# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Recode** is an open-source, provider-agnostic alternative to Claude Code.

Claude Code is excellent - its interactive CLI experience, tool integration, and agent loop design are impressive. However, it's locked to Anthropic's Claude models. Recode aims to bring that same great experience while giving users the freedom to switch between different LLM providers freely.

### Core Goals

- **Provider Freedom**: Support OpenAI, Anthropic, Google Gemini, and any OpenAI-compatible API
- **Same Great Experience**: Replicate Claude Code's intuitive CLI workflow and tool integration
- **No Vendor Lock-in**: Use any cloud provider or local models
- **Open & Extensible**: Fully open-source and customizable

### Key Components

- **Agent System**: Multi-turn conversation with tool calling loop
- **Built-in Tools**: Bash, Read, Write, Edit, Glob, Grep, and more
- **Provider Abstraction**: Unified interface for multiple LLM providers
- **Permission System**: Configurable auto/confirm/deny for tool execution
- **Interactive CLI**: Streaming output with rich terminal interface

## Development Approach

- Analyze Claude Code's patterns and replicate the best aspects
- Implement provider-agnostic abstractions
- Maintain compatibility with Claude Code tool schemas where practical
- Use conventional commit format for commit messages
