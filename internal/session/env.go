package session

import (
	"os/exec"
	"strings"
)

func GetGitBranch(dir string) string {
	return getGitBranch(dir)
}

func getGitBranch(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
