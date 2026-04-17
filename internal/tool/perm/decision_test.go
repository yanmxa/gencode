package perm

import "testing"

func TestIsReadOnlyTool(t *testing.T) {
	readOnly := []string{"Read", "Glob", "Grep", "WebFetch", "WebSearch", "LSP"}
	for _, name := range readOnly {
		if !IsReadOnlyTool(name) {
			t.Errorf("IsReadOnlyTool(%q) = false, want true", name)
		}
	}

	notReadOnly := []string{"Bash", "Write", "Edit", "Agent"}
	for _, name := range notReadOnly {
		if IsReadOnlyTool(name) {
			t.Errorf("IsReadOnlyTool(%q) = true, want false", name)
		}
	}
}

func TestIsSafeTool(t *testing.T) {
	safe := []string{"TaskCreate", "TaskGet", "TaskList", "TaskUpdate",
		"AskUserQuestion", "EnterPlanMode", "ExitPlanMode", "ToolSearch", "LSP"}
	for _, name := range safe {
		if !IsSafeTool(name) {
			t.Errorf("IsSafeTool(%q) = false, want true", name)
		}
	}

	notSafe := []string{"Edit", "Bash", "Write", "Agent"}
	for _, name := range notSafe {
		if IsSafeTool(name) {
			t.Errorf("IsSafeTool(%q) = true, want false", name)
		}
	}
}

func TestCheckers(t *testing.T) {
	t.Run("PermitAll", func(t *testing.T) {
		c := PermitAll()
		if d := c.Check("Bash", nil); d != Permit {
			t.Errorf("PermitAll.Check(Bash) = %v, want Permit", d)
		}
	})

	t.Run("DenyAll", func(t *testing.T) {
		c := DenyAll()
		if d := c.Check("Read", nil); d != Reject {
			t.Errorf("DenyAll.Check(Read) = %v, want Reject", d)
		}
	})

	t.Run("ReadOnly permits reads", func(t *testing.T) {
		c := ReadOnly()
		for _, name := range []string{"Read", "Glob", "Grep"} {
			if d := c.Check(name, nil); d != Permit {
				t.Errorf("ReadOnly.Check(%s) = %v, want Permit", name, d)
			}
		}
	})

	t.Run("ReadOnly rejects writes", func(t *testing.T) {
		c := ReadOnly()
		for _, name := range []string{"Write", "Edit", "Bash"} {
			if d := c.Check(name, nil); d != Reject {
				t.Errorf("ReadOnly.Check(%s) = %v, want Reject", name, d)
			}
		}
	})

	t.Run("AcceptEdits permits edits", func(t *testing.T) {
		c := AcceptEdits()
		for _, name := range []string{"Edit", "Write", "NotebookEdit", "Read"} {
			if d := c.Check(name, nil); d != Permit {
				t.Errorf("AcceptEdits.Check(%s) = %v, want Permit", name, d)
			}
		}
	})

	t.Run("AcceptEdits prompts others", func(t *testing.T) {
		c := AcceptEdits()
		if d := c.Check("Bash", nil); d != Prompt {
			t.Errorf("AcceptEdits.Check(Bash) = %v, want Prompt", d)
		}
	})
}

func TestAsPermissionFunc(t *testing.T) {
	t.Run("nil checker returns nil", func(t *testing.T) {
		if fn := AsPermissionFunc(nil); fn != nil {
			t.Error("AsPermissionFunc(nil) should return nil")
		}
	})

	t.Run("reject returns false", func(t *testing.T) {
		fn := AsPermissionFunc(DenyAll())
		allow, reason := fn(nil, "Bash", nil)
		if allow {
			t.Error("expected deny")
		}
		if reason == "" {
			t.Error("expected non-empty reason")
		}
	})

	t.Run("permit returns true", func(t *testing.T) {
		fn := AsPermissionFunc(PermitAll())
		allow, _ := fn(nil, "Bash", nil)
		if !allow {
			t.Error("expected allow")
		}
	})
}
