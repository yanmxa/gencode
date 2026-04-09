// Package registry imports all tool sub-packages to trigger their init() registration.
package registry

import (
	_ "github.com/yanmxa/gencode/internal/tool/agent"
	_ "github.com/yanmxa/gencode/internal/tool/cron"
	_ "github.com/yanmxa/gencode/internal/tool/fs"
	_ "github.com/yanmxa/gencode/internal/tool/mode"
	_ "github.com/yanmxa/gencode/internal/tool/skill"
	_ "github.com/yanmxa/gencode/internal/tool/task"
	_ "github.com/yanmxa/gencode/internal/tool/tasktools"
	_ "github.com/yanmxa/gencode/internal/tool/web"
)
