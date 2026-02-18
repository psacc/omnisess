package detect

import (
	"os"
	"os/exec"
	"strings"
	"time"
)

// processPatterns maps tool names to pgrep patterns.
var processPatterns = map[string]string{
	"claude": "claude",
	"cursor": "Cursor",
	"codex":  "codex",
	"gemini": "gemini",
}

// IsToolRunning checks if a tool's process is currently running.
func IsToolRunning(toolName string) bool {
	pattern, ok := processPatterns[toolName]
	if !ok {
		return false
	}
	cmd := exec.Command("pgrep", "-f", pattern)
	err := cmd.Run()
	return err == nil
}

// IsFileRecentlyModified returns true if the file was modified within the given threshold.
func IsFileRecentlyModified(path string, threshold time.Duration) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < threshold
}

// IsSessionActive returns true if the tool is running AND the session file was recently modified.
func IsSessionActive(toolName string, sessionFilePath string) bool {
	if !IsToolRunning(toolName) {
		return false
	}
	return IsFileRecentlyModified(sessionFilePath, 2*time.Minute)
}

// Truncate returns s truncated to maxLen with "..." appended if needed.
func Truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	// Replace newlines with spaces for preview
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
