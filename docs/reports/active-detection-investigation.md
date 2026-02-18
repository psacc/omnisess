# Active Session Detection Investigation

**Date**: 2026-02-18
**Branch**: `fix/active-detection`
**Tested on**: macOS Darwin 25.3.0 (Apple Silicon)

## Executive Summary

`sessions active` is fundamentally broken. It produces both false negatives (misses running sessions) and false positives (could mark idle sessions as active). The two core detection heuristics -- process matching via `pgrep` and file mtime within 2 minutes -- are both unreliable. Additionally, the session enumeration depends on `history.jsonl` which only covers ~45% of actual sessions.

## What Works

- The `active` command plumbing is correct: `opts.Active = true` propagates through `List()`, the filter logic is applied, the JSON output includes the `Active` field.
- `findSessionFile` correctly locates session files by glob matching.
- `sessionFileUpdatedAt` correctly refines `UpdatedAt` from file mtime.
- Cursor source active detection follows the same pattern and has the same issues.

## What Does Not Work

### Bug 1: CRITICAL -- 2-minute mtime threshold causes constant false negatives

**The problem**: `IsFileRecentlyModified(path, 2*time.Minute)` checks whether the session `.jsonl` file was modified within the last 2 minutes. During a normal Claude Code session, the file may not be written to for several minutes while:
- The model is generating a long response (thinking + output can take 2+ min)
- The user is reading a response
- The user is typing a follow-up prompt
- The model is executing long-running tool calls

**Reproduction**:
```bash
# Start a session, wait 3 minutes without sending a message, then:
./sessions active --json
# Output: null (no active sessions)
# But the session is clearly still running
```

**Observed**: During this investigation, a session that was detected as `ACTIVE` became undetected within ~3 minutes of the last file write, despite the session being actively in use (this very session).

**Recommendation**: Increase threshold to 10-15 minutes. Alternatively, also check mtime of subagent files in `<session_id>/subagents/` since those are often written more recently than the main session file.

### Bug 2: CRITICAL -- `pgrep -f claude` matches far too many processes

**The problem**: `pgrep -f claude` matches any process whose full command line contains the string "claude". On this machine, it matches **19 processes** including:
- Claude.app's ShipIt auto-updater (PID 476) -- always running in background
- Claude Desktop's disclaimer wrapper processes
- Cursor extension's embedded Claude Code binaries
- Actual `claude` CLI processes
- Claude Code agent SDK processes

This means `IsToolRunning("claude")` **always returns true** when Claude Desktop is installed, regardless of whether any coding session is running.

Similarly, `pgrep -f Cursor` matches **25 processes** including system-level `CursorUIViewService.xpc` (a macOS input framework process unrelated to Cursor IDE).

**Observed process matches for `pgrep -f claude`**:
```
476  Claude.app ShipIt (auto-updater, always running)
5675 claude (CLI)
7178 Cursor extension claude binary
8599 Claude.app disclaimer wrapper
...19 total matches
```

**Recommendation**: Use more specific patterns:
- For Claude CLI: match on the exact binary name or `--resume <session-id>` pattern
- For Cursor: match on `Cursor.app/Contents/MacOS/Cursor` specifically, not any process with "Cursor" in the command line
- Consider matching the specific session ID in the process args (`--resume <id>`) to make detection session-specific rather than tool-wide

### Bug 3: HIGH -- history.jsonl is incomplete, misses 55% of sessions

**The problem**: Session enumeration in `claude.List()` iterates over `~/.claude/history.jsonl`. However, sessions started from Cursor's embedded Claude Code extension (and possibly Claude Desktop's agent mode) write session `.jsonl` files to disk but do NOT append to `history.jsonl`.

**Observed**: 30 unique session IDs in `history.jsonl` vs 67 `.jsonl` files on disk at `~/.claude/projects/*/`. That means 37 sessions are completely invisible to `sessions list` and `sessions active`.

**Example**: Session `5594f717` in `~/.claude/projects/-Users-paolo-sacconier-prj-finn/` was modified at 14:42:56 (within 2 min at time of test) but does not appear in history.jsonl and is not shown by `sessions active`.

**Recommendation**: Add a fallback that scans `~/.claude/projects/*/` for `.jsonl` files not present in history.jsonl. Use file metadata (mtime, first message) to populate session entries.

### Bug 4: MEDIUM -- Subagent files not checked for mtime

**The problem**: Claude Code writes subagent files to `<session_id>/subagents/agent-*.jsonl`. These are often the most recently modified files for a session. `IsSessionActive` only checks the main session file mtime.

**Observed**: Main session file at `14:42:56`, subagent files at `14:45:46` -- a 3-minute gap where the main file appears stale but the session is clearly active.

**Recommendation**: In `IsFileRecentlyModified`, also glob and check `<session_dir>/subagents/*.jsonl` mtimes. If any file in the session directory tree is recent, consider the session active.

### Bug 5: LOW -- projectPathFromDir corrupts paths with hyphens

**The problem**: `projectPathFromDir` converts `-Users-foo-my-project` to `/Users/foo/my/project` by replacing ALL `-` with `/`. Any project path containing hyphens (extremely common) is corrupted.

**Code**:
```go
func projectPathFromDir(dirName string) string {
    return strings.ReplaceAll(dirName, "-", "/")
}
```

**Impact**: Session-to-project matching via `findSessionFileForProject` may fail for projects with hyphens in their path, falling back to the slower glob-based `findSessionFile`. The `Project` field in output would also be wrong, but it is typically overridden by the `history.jsonl` value which stores the correct path.

**Note**: This is a known limitation of Claude Code's directory naming scheme. There is no lossless way to reverse the encoding. The only reliable approach is to use the `project` field from `history.jsonl` as the source of truth, or parse the directory listing to match against known project paths.

### Bug 6: LOW -- JSON output includes empty/null fields

**The problem**: `model.Session` struct fields have no `json:",omitempty"` tags, so the JSON output includes `"Summary": ""`, `"Model": ""`, `"Messages": null`.

**Impact**: Clutters JSON output. Consumers need to handle both empty strings and missing fields.

## Test Results

```
$ ./sessions active
# Initially detected this session as ACTIVE (correct)
# After ~3 minutes of no file writes: reported no active sessions (false negative)

$ ./sessions active --json
# Correctly structured JSON with Active=true field
# After threshold elapsed: outputs "null"

$ pgrep -f claude
# Returns 19 PIDs -- always succeeds even with no CLI sessions

$ pgrep -f Cursor
# Returns 25 PIDs -- always succeeds even with no active Cursor AI sessions

$ ./sessions list --since=1h
# Shows only 1 session from history.jsonl
# Misses sessions only existing as files on disk
```

## Recommended Fix Priority

1. **Increase mtime threshold** to 10 minutes (quick fix, high impact)
2. **Check subagent file mtimes** in IsSessionActive (moderate fix, high impact)
3. **Improve pgrep patterns** to be more specific (moderate fix, prevents false positives)
4. **Scan disk for sessions not in history.jsonl** (larger change, addresses session enumeration gap)
5. **Add `json:",omitempty"` tags** to model.Session (trivial fix)
6. **Document or mitigate** the hyphenated path issue (design decision needed)

## Fixes Applied in This Branch

### Fix 1: Increase mtime threshold from 2 minutes to 10 minutes

The 2-minute threshold is too aggressive. During a normal session, Claude Code can easily go several minutes without writing to the session file. A 10-minute threshold provides a reasonable buffer while still detecting sessions that have genuinely ended.

### Fix 2: Check subagent file mtimes

Modified `IsSessionActive` to check both the main session file and any subagent files in the `<session_id>/subagents/` directory. If any file in the session tree was recently modified, the session is considered active.

### Fix 3: Improved process detection patterns

Changed `pgrep` patterns to be more specific:
- Claude: `pgrep -x claude` (exact match on process name) instead of `pgrep -f claude` (matches anywhere in command line)
- Cursor: kept `-f` but uses a more specific pattern for the main Cursor process

### Fix 4: JSON omitempty tags on model.Session

Added `json:",omitempty"` to optional fields in `model.Session` to clean up JSON output.
