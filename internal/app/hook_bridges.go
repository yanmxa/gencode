package app

import (
	"strings"

	"github.com/yanmxa/gencode/internal/ext/mcp"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/worktree"
)

type taskHookBridge struct {
	engine            *hooks.Engine
	taskNotifications *taskNotificationQueue
}

func (b taskHookBridge) TaskCreated(info task.TaskInfo) {
	if b.engine == nil {
		return
	}
	b.engine.ExecuteAsync(hooks.TaskCreated, hooks.HookInput{
		TaskID:          info.ID,
		TaskSubject:     taskSubject(info),
		TaskDescription: info.Description,
	})
}

func (b taskHookBridge) TaskCompleted(info task.TaskInfo) {
	if b.engine != nil {
		b.engine.ExecuteAsync(hooks.TaskCompleted, hooks.HookInput{
			TaskID:          info.ID,
			TaskSubject:     taskSubject(info),
			TaskDescription: info.Description,
		})
	}

	updateBackgroundWorkerTracker(info)
	if b.taskNotifications == nil {
		return
	}
	if item, ok := buildTaskNotification(info); ok {
		b.taskNotifications.Push(item)
	}
}

type worktreeHookBridge struct {
	engine *hooks.Engine
}

type configHookBridge struct {
	engine *hooks.Engine
}

func (b worktreeHookBridge) WorktreeCreated(name, path string) {
	if b.engine == nil {
		return
	}
	b.engine.ExecuteAsync(hooks.WorktreeCreate, hooks.HookInput{
		Name:         name,
		WorktreePath: path,
	})
}

func (b worktreeHookBridge) WorktreeRemoved(path string) {
	if b.engine == nil {
		return
	}
	b.engine.ExecuteAsync(hooks.WorktreeRemove, hooks.HookInput{
		WorktreePath: path,
	})
}

func (b configHookBridge) ConfigChanged(source, filePath string) {
	if b.engine == nil {
		return
	}
	b.engine.ExecuteAsync(hooks.ConfigChange, hooks.HookInput{
		Source:   source,
		FilePath: filePath,
	})
	b.engine.ExecuteAsync(hooks.FileChanged, hooks.HookInput{
		Source:   source,
		FilePath: filePath,
	})
}

func joinNameDesc(name, desc string) string {
	name = strings.TrimSpace(name)
	desc = strings.TrimSpace(desc)
	switch {
	case name != "" && desc != "" && !strings.EqualFold(name, desc):
		return name + ": " + desc
	case desc != "":
		return desc
	case name != "":
		return name
	default:
		return ""
	}
}

func taskSubject(info task.TaskInfo) string {
	switch info.Type {
	case task.TaskTypeAgent:
		if s := joinNameDesc(info.AgentName, info.Description); s != "" {
			return s
		}
	case task.TaskTypeBash:
		if info.Command != "" {
			return info.Command
		}
	}
	return info.Description
}

func installHookBridges(engine *hooks.Engine, taskNotifications *taskNotificationQueue) {
	task.SetHookObserver(taskHookBridge{engine: engine, taskNotifications: taskNotifications})
	worktree.SetHookObserver(worktreeHookBridge{engine: engine})
	plugin.SetConfigObserver(configHookBridge{engine: engine})
	mcp.SetConfigObserver(configHookBridge{engine: engine})
}
