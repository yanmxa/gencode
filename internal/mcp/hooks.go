package mcp

import "github.com/yanmxa/gencode/internal/hook"

func fireConfigChanged(source, filePath string) {
	if hook.DefaultEngine != nil {
		hook.DefaultEngine.ExecuteAsync(hook.ConfigChange, hook.HookInput{
			Source:   source,
			FilePath: filePath,
		})
		hook.DefaultEngine.ExecuteAsync(hook.FileChanged, hook.HookInput{
			Source:   source,
			FilePath: filePath,
		})
	}
}
