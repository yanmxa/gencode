package plugin

import "github.com/yanmxa/gencode/internal/hook"

func fireConfigChanged(source, filePath string) {
	if h := hook.DefaultIfInit(); h != nil {
		h.ExecuteAsync(hook.ConfigChange, hook.HookInput{
			Source:   source,
			FilePath: filePath,
		})
		h.ExecuteAsync(hook.FileChanged, hook.HookInput{
			Source:   source,
			FilePath: filePath,
		})
	}
}
