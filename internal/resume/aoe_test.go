//go:build !windows

package resume

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// writeFakeAoE writes a shell script named "aoe" to binDir that implements
// the subset of the aoe CLI used by FindAoESession and ExecInAoE.
// The script outputs the provided sessions JSON when called with "list --json"
// and exits 0 for all other subcommands.
func writeFakeAoE(t *testing.T, binDir string, sessionsJSON string) string {
	t.Helper()
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"list\" ]; then printf '%%s' '%s'; exit 0; fi\nexit 0\n", sessionsJSON)
	aoePath := filepath.Join(binDir, "aoe")
	if err := os.WriteFile(aoePath, []byte(script), 0o755); err != nil {
		t.Fatalf("could not write fake aoe: %v", err)
	}
	return aoePath
}

// writeSelfTruncatingAoE writes an "aoe" script that:
//   - On the first invocation (list --json): outputs sessionsJSON and then
//     truncates itself to zero bytes.
//   - On subsequent invocations (e.g. via syscall.Exec): the kernel rejects the
//     zero-byte binary with ENOEXEC, causing syscall.Exec to return an error
//     instead of replacing the process.
//
// This lets tests reach and cover the return syscall.Exec(...) statement while
// preserving coverage data (the process is not replaced).
func writeSelfTruncatingAoE(t *testing.T, binDir string, sessionsJSON string) {
	t.Helper()
	// The script outputs the JSON for "list --json" then truncates itself.
	// Any subsequent exec will fail with ENOEXEC (exec format error).
	script := fmt.Sprintf(
		"#!/bin/sh\n"+
			"if [ \"$1\" = \"list\" ]; then\n"+
			"  printf '%%s' '%s'\n"+
			"  : > \"$0\"\n"+ // truncate self to 0 bytes
			"  exit 0\n"+
			"fi\n"+
			"exit 0\n",
		sessionsJSON,
	)
	aoePath := filepath.Join(binDir, "aoe")
	if err := os.WriteFile(aoePath, []byte(script), 0o755); err != nil {
		t.Fatalf("could not write self-truncating aoe: %v", err)
	}
}

// writeEmptyExec writes a zero-byte executable file named name to binDir.
// exec.LookPath finds it (executable bit set), but syscall.Exec fails with
// "exec format error" — allowing coverage of the post-LookPath statements
// while keeping the test process alive.
func writeEmptyExec(t *testing.T, binDir, name string) {
	t.Helper()
	p := filepath.Join(binDir, name)
	if err := os.WriteFile(p, []byte{}, 0o755); err != nil {
		t.Fatalf("could not write empty exec %s: %v", name, err)
	}
}

// TestFindAoESession_MissingBinary exercises the LookPath-not-found error path.
func TestFindAoESession_MissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := FindAoESession("claude", "/tmp/project")
	if err == nil {
		t.Fatal("FindAoESession returned nil when aoe is not in PATH, want error")
	}
}

// TestFindAoESession_CommandFails exercises the error path when "aoe list --json" fails.
func TestFindAoESession_CommandFails(t *testing.T) {
	binDir := t.TempDir()
	// Empty executable: LookPath succeeds, exec.Command fails with exec format error.
	writeEmptyExec(t, binDir, "aoe")
	t.Setenv("PATH", binDir)

	_, err := FindAoESession("claude", "/tmp/project")
	if err == nil {
		t.Fatal("FindAoESession returned nil when aoe command fails, want error")
	}
}

// TestFindAoESession_InvalidJSON exercises the JSON unmarshal error path.
func TestFindAoESession_InvalidJSON(t *testing.T) {
	binDir := t.TempDir()
	script := "#!/bin/sh\nprintf 'not-valid-json'\n"
	if err := os.WriteFile(filepath.Join(binDir, "aoe"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	_, err := FindAoESession("claude", "/tmp/project")
	if err == nil {
		t.Fatal("FindAoESession returned nil for invalid JSON output, want error")
	}
}

// TestFindAoESession_NoSessions exercises the path where the session list is
// empty — the function should return ("", nil).
func TestFindAoESession_NoSessions(t *testing.T) {
	binDir := t.TempDir()
	writeFakeAoE(t, binDir, "[]")
	t.Setenv("PATH", binDir)

	id, err := FindAoESession("claude", "/tmp/project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "" {
		t.Errorf("FindAoESession() = %q, want empty string", id)
	}
}

// TestFindAoESession_ToolMismatch exercises the loop body where s.Tool != tool.
func TestFindAoESession_ToolMismatch(t *testing.T) {
	binDir := t.TempDir()
	sessions := []aoeSession{{ID: "sess1", Tool: "cursor", ProjectPath: "/some/path"}}
	data, _ := json.Marshal(sessions)
	writeFakeAoE(t, binDir, string(data))
	t.Setenv("PATH", binDir)

	id, err := FindAoESession("claude", "/some/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "" {
		t.Errorf("FindAoESession() = %q, want empty string for tool mismatch", id)
	}
}

// TestFindAoESession_MatchFound exercises the happy path where a session matches.
func TestFindAoESession_MatchFound(t *testing.T) {
	dir := t.TempDir()
	binDir := t.TempDir()

	sessions := []aoeSession{{ID: "sess42", Tool: "claude", ProjectPath: dir}}
	data, _ := json.Marshal(sessions)
	writeFakeAoE(t, binDir, string(data))
	t.Setenv("PATH", binDir)

	id, err := FindAoESession("claude", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "sess42" {
		t.Errorf("FindAoESession() = %q, want %q", id, "sess42")
	}
}

// TestFindAoESession_SymlinkProjectPath exercises the filepath.EvalSymlinks
// fallback for paths that cannot be resolved, testing that the raw path is used.
func TestFindAoESession_SymlinkProjectPath(t *testing.T) {
	binDir := t.TempDir()

	// Use a nonexistent path so EvalSymlinks fails for both sides; both fall
	// back to raw path comparison and should match.
	nonExistent := "/nonexistent_path_xyz_omnisess_test_42"
	sessions := []aoeSession{{ID: "sess9", Tool: "claude", ProjectPath: nonExistent}}
	data, _ := json.Marshal(sessions)
	writeFakeAoE(t, binDir, string(data))
	t.Setenv("PATH", binDir)

	id, err := FindAoESession("claude", nonExistent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "sess9" {
		t.Errorf("FindAoESession() = %q, want %q", id, "sess9")
	}
}

// TestExecInAoE_MissingBinary exercises the LookPath-not-found error path.
func TestExecInAoE_MissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	err := ExecInAoE("claude", "/tmp/project", "test title")
	if err == nil {
		t.Fatal("ExecInAoE returned nil when aoe is not in PATH, want error")
	}
}

// TestExecInAoE_NewSession exercises the path where no existing AoE session is
// found and ExecInAoE builds a "new session" argv. The empty executable makes
// syscall.Exec return an error, preserving coverage data.
func TestExecInAoE_NewSession(t *testing.T) {
	binDir := t.TempDir()
	// Empty executable: LookPath succeeds; FindAoESession's exec.Command fails
	// (exec format error), so existingID="" and ExecInAoE takes the else branch.
	// syscall.Exec also fails on the empty binary, so coverage data is preserved.
	writeEmptyExec(t, binDir, "aoe")
	t.Setenv("PATH", binDir)

	err := ExecInAoE("claude", t.TempDir(), "My Project (claude)")
	if err == nil {
		t.Fatal("ExecInAoE with empty executable returned nil, want error")
	}
}

// TestExecInAoE_ExistingSession exercises the path where FindAoESession returns
// an existing session ID, causing ExecInAoE to take the "attach" argv branch.
//
// The self-truncating aoe script outputs valid JSON for "list --json" and then
// truncates itself to 0 bytes. When ExecInAoE subsequently calls
// syscall.Exec(aoePath, ...), the kernel rejects the zero-byte file with
// ENOEXEC, returning an error instead of replacing the process. This keeps the
// test process alive so coverage data is preserved.
func TestExecInAoE_ExistingSession(t *testing.T) {
	projectDir := t.TempDir()
	binDir := t.TempDir()

	sessions := []aoeSession{{ID: "exist-sess", Tool: "claude", ProjectPath: projectDir}}
	data, _ := json.Marshal(sessions)
	writeSelfTruncatingAoE(t, binDir, string(data))
	t.Setenv("PATH", binDir)

	// ExecInAoE will:
	// 1. LookPath("aoe") → finds the script
	// 2. FindAoESession → runs "aoe list --json" → gets JSON → returns "exist-sess"
	//    (script truncates itself to 0 bytes after outputting JSON)
	// 3. existingID = "exist-sess" ≠ "" → takes the attach branch (Block D)
	// 4. syscall.Exec(aoePath, ...) → file is now 0 bytes → ENOEXEC → returns error
	err := ExecInAoE("claude", projectDir, "test (claude)")
	if err == nil {
		t.Fatal("ExecInAoE with self-truncating aoe returned nil, want error (exec format error)")
	}
}
