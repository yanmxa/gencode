package tui

// Import all provider packages to trigger their init() functions
// which register the providers with the global registry.

import (
	// Import provider packages for side effects (registration)
	_ "github.com/yanmxa/gencode/internal/provider/anthropic"
	_ "github.com/yanmxa/gencode/internal/provider/google"
	_ "github.com/yanmxa/gencode/internal/provider/openai"
)
