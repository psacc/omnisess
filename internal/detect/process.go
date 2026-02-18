package detect

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ActiveThreshold is how recently a session file must have been modified
// to be considered active. 10 minutes accounts for long model responses,
// user reading time, and gaps between tool calls.
const ActiveThreshold = 10 * time.Minute

// IsToolRunning checks if a tool's process is currently running.
// Uses exact process name matching (-x) for claude to avoid matching
// Claude.app, ShipIt, browser tabs, etc. Falls back to -f for tools
// where the process name differs from the pattern.
func IsToolRunning(toolName string) bool {
	switch toolName {
	case "claude":
		// Use -x for exact process name match. This avoids matching
		// Claude.app, ShipIt updater, Cursor extension processes, etc.
		// The CLI binary is literally named "claude".
		cmd := exec.Command("pgrep", "-x", "claude")
		return cmd.Run() == nil
	case "cursor":
		// Match the main Cursor Electron process specifically.
		cmd := exec.Command("pgrep", "-f", "Cursor.app/Contents/MacOS/Cursor")
		return cmd.Run() == nil
	case "codex":
		cmd := exec.Command("pgrep", "-x", "codex")
		return cmd.Run() == nil
	case "gemini":
		cmd := exec.Command("pgrep", "-f", "gemini")
		return cmd.Run() == nil
	default:
		return false
	}
}

// IsFileRecentlyModified returns true if the file was modified within the given threshold.
func IsFileRecentlyModified(path string, threshold time.Duration) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < threshold
}

// isSessionTreeRecentlyModified checks whether the session file OR any of its
// subagent files were recently modified. Claude Code writes subagent transcripts
// to <session_id>/subagents/agent-*.jsonl, and these are often the most recently
// modified files during an active session.
func isSessionTreeRecentlyModified(sessionFilePath string, threshold time.Duration) bool {
	// Check the main session file first.
	if IsFileRecentlyModified(sessionFilePath, threshold) {
		return true
	}

	// Check subagent files: <session_id_without_ext>/subagents/*.jsonl
	sessionDir := strings.TrimSuffix(sessionFilePath, ".jsonl")
	subagentPattern := filepath.Join(sessionDir, "subagents", "*.jsonl")
	matches, err := filepath.Glob(subagentPattern)
	if err != nil || len(matches) == 0 {
		return false
	}

	for _, m := range matches {
		if IsFileRecentlyModified(m, threshold) {
			return true
		}
	}
	return false
}

// IsSessionActive returns true if the tool is running AND the session file
// (or its subagent files) was recently modified.
func IsSessionActive(toolName string, sessionFilePath string) bool {
	if !IsToolRunning(toolName) {
		return false
	}
	return isSessionTreeRecentlyModified(sessionFilePath, ActiveThreshold)
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
