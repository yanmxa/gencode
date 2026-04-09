package task

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestAgentTaskInitializesStableOutputFile(t *testing.T) {
	tmpDir := t.TempDir()
	if err := SetOutputDir(tmpDir); err != nil {
		t.Fatalf("SetOutputDir() error: %v", err)
	}
	t.Cleanup(func() {
		_ = SetOutputDir("")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	task := NewAgentTask("task-output-1", "Explore", "Inspect code", ctx, cancel)
	if task.OutputFile == "" {
		t.Fatal("expected output file path to be assigned")
	}
	if _, err := os.Stat(task.OutputFile); err != nil {
		t.Fatalf("expected output file to exist: %v", err)
	}
}

func TestAgentTaskAppendOutputWritesToOutputFile(t *testing.T) {
	tmpDir := t.TempDir()
	if err := SetOutputDir(tmpDir); err != nil {
		t.Fatalf("SetOutputDir() error: %v", err)
	}
	t.Cleanup(func() {
		_ = SetOutputDir("")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	task := NewAgentTask("task-output-2", "Explore", "Inspect code", ctx, cancel)
	task.AppendOutput([]byte("first line\n"))
	task.AppendProgress("Read(main.go)")

	data, err := os.ReadFile(task.OutputFile)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	text := string(data)
	for _, want := range []string{`"event":"task.started"`, `"event":"task.output"`, `"event":"task.progress"`, "first line", "Read(main.go)"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output file missing %q: %q", want, text)
		}
	}
}
