package setting

import "os"

// IsGitRepo checks if the given directory is a git repository.
func IsGitRepo(dir string) bool {
	_, err := os.Stat(dir + "/.git")
	return err == nil
}
