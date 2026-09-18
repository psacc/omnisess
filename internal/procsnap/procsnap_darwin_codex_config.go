//go:build darwin

package procsnap

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

var warnW io.Writer = os.Stderr

var codexSessionsDirFn = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex", "sessions"), nil
}

var codexLsofFn = func(pids []int) ([]byte, error) {
	strs := make([]string, len(pids))
	for i, pid := range pids {
		strs[i] = strconv.Itoa(pid)
	}
	return exec.Command("lsof", "-n", "-P", "-a", "-p", strings.Join(strs, ","), "-F", "pfn").Output()
}

var codexMetaMaxLine = 4 * 1024 * 1024
var codexLsofMaxLine = 1024 * 1024

func codexDirs() (string, string, error) {
	sessionsDir, err := codexSessionsDirFn()
	if err != nil {
		return "", "", err
	}
	lockDir := filepath.Join(filepath.Dir(sessionsDir), "thread-writer-locks")
	if resolved, resolveErr := filepath.EvalSymlinks(lockDir); resolveErr == nil {
		lockDir = resolved
	}
	if resolved, resolveErr := filepath.EvalSymlinks(sessionsDir); resolveErr == nil {
		sessionsDir = resolved
	}
	return sessionsDir, lockDir, nil
}

func lsofErrDetail(err error) string {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		return ": " + strings.TrimSpace(string(exitErr.Stderr))
	}
	return ""
}
