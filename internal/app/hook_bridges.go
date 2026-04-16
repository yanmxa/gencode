package app

import (
	appagent "github.com/yanmxa/gencode/internal/app/agent"
	"github.com/yanmxa/gencode/internal/extension/mcp"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/extension/plugin"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/worktree"
)

type taskHookBridge struct {
	engine        *hooks.Engine
	notifications *appagent.NotificationQueue
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

	appagent.UpdateBackgroundWorkerTracker(info)
	if b.notifications == nil {
		return
	}
	notifInput := appagent.TaskNotificationInput{
		Info:    info,
		Subject: taskSubject(info),
		Batch:   appagent.SnapshotBackgroundBatchForTask(info.ID),
	}
	if item, ok := appagent.BuildTaskNotification(notifInput); ok {
		b.notifications.Push(item)
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

func taskSubject(info task.TaskInfo) string {
	switch info.Type {
	case task.TaskTypeAgent:
		if s := appagent.JoinNameDesc(info.AgentName, info.Description); s != "" {
			return s
		}
	case task.TaskTypeBash:
		if info.Command != "" {
			return info.Command
		}
	}
	return info.Description
}

func installHookBridges(engine *hooks.Engine, notifications *appagent.NotificationQueue) {
	task.SetHookObserver(taskHookBridge{engine: engine, notifications: notifications})
	worktree.SetHookObserver(worktreeHookBridge{engine: engine})
	plugin.SetConfigObserver(configHookBridge{engine: engine})
	mcp.SetConfigObserver(configHookBridge{engine: engine})
}
