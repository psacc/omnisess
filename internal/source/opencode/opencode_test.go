package opencode

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/psacc/omnisess/internal/model"
	"github.com/psacc/omnisess/internal/source"
)

// baseTime is the newest fixture timestamp: several tests pin nowFn here so
// Active/Since filtering is deterministic.
var baseTime = time.UnixMilli(1700000600000).UTC()

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func execScript(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	script, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if _, err := db.Exec(string(script)); err != nil {
		t.Fatalf("exec %s: %v", name, err)
	}
}

// setupDB creates a fake HOME whose OpenCode database is built from the given
// schema/data scripts. It returns the database path.
func setupDB(t *testing.T, schema, data string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")

	dir := filepath.Join(home, ".local", "share", "opencode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "opencode.db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	execScript(t, db, schema)
	if data != "" {
		execScript(t, db, data)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// stubRunner pins the process probe so tests are independent of whether an
// opencode process happens to be running on the host.
func stubRunner(t *testing.T, running bool) {
	t.Helper()
	orig := isToolRunning
	isToolRunning = func(string) bool { return running }
	t.Cleanup(func() { isToolRunning = orig })
}

func setupV2(t *testing.T) string {
	t.Helper()
	return setupDB(t, "schema_v2.sql", "data_v2.sql")
}

func setupLegacy(t *testing.T) string {
	t.Helper()
	return setupDB(t, "schema_legacy.sql", "data_legacy.sql")
}

func sessionIDs(sessions []model.Session) []string {
	ids := make([]string, len(sessions))
	for i, s := range sessions {
		ids[i] = s.ID
	}
	return ids
}

// ---------------------------------------------------------------------------
// Name / paths / model labels
// ---------------------------------------------------------------------------

func TestName(t *testing.T) {
	s := &opencodeSource{}
	if s.Name() != model.ToolOpenCode {
		t.Errorf("Name() = %q, want %q", s.Name(), model.ToolOpenCode)
	}
}

func TestDataDir(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	if got, want := dataDir("/home/foo"), "/home/foo/.local/share/opencode"; got != want {
		t.Errorf("dataDir() = %q, want %q", got, want)
	}

	t.Setenv("XDG_DATA_HOME", "/xdg/data")
	if got, want := dataDir("/home/foo"), "/xdg/data/opencode"; got != want {
		t.Errorf("dataDir() with XDG_DATA_HOME = %q, want %q", got, want)
	}
}

func TestDBPath(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	want := filepath.Join("/home/foo", ".local", "share", "opencode", "opencode.db")
	if got := dbPath("/home/foo"); got != want {
		t.Errorf("dbPath() = %q, want %q", got, want)
	}
}

func TestModelLabel(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"provider and id", `{"id":"gpt","providerID":"openai"}`, "openai/gpt"},
		{"legacy modelID", `{"modelID":"glm","providerID":"scaleway"}`, "scaleway/glm"},
		{"id only", `{"id":"gpt"}`, "gpt"},
		{"modelID only", `{"modelID":"glm"}`, "glm"},
		{"neither", `{"providerID":"openai"}`, ""},
		{"malformed", `not-json`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := modelLabel(tt.raw); got != tt.want {
				t.Errorf("modelLabel(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestUnixMilli(t *testing.T) {
	if got := unixMilli(0); !got.IsZero() {
		t.Errorf("unixMilli(0) = %v, want zero", got)
	}
	if got, want := unixMilli(1700000000000), time.UnixMilli(1700000000000).UTC(); !got.Equal(want) {
		t.Errorf("unixMilli(1700000000000) = %v, want %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// Schema helpers
// ---------------------------------------------------------------------------

func TestHasTableAndUseV2Schema(t *testing.T) {
	path := setupV2(t)
	db := openReadOnly(path)
	defer db.Close()

	ok, err := hasTable(db, "session_v2")
	if err != nil || !ok {
		t.Fatalf("hasTable(session_v2) = %v, %v; want true, nil", ok, err)
	}
	ok, err = hasTable(db, "does_not_exist")
	if err != nil || ok {
		t.Fatalf("hasTable(missing) = %v, %v; want false, nil", ok, err)
	}
	v2, err := useV2Schema(db)
	if err != nil || !v2 {
		t.Fatalf("useV2Schema() = %v, %v; want true, nil", v2, err)
	}
}

func TestHasTable_ClosedDB(t *testing.T) {
	path := setupV2(t)
	db := openReadOnly(path)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := hasTable(db, "session_v2"); err == nil {
		t.Fatal("expected error from closed DB, got nil")
	}
	if _, err := useV2Schema(db); err == nil {
		t.Fatal("expected error from closed DB, got nil")
	}
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestList_HomeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	if _, err := (&opencodeSource{}).List(source.ListOptions{}); err == nil {
		t.Fatal("expected error when HOME is empty, got nil")
	}
}

func TestList_MissingDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")

	sessions, err := (&opencodeSource{}).List(source.ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if sessions != nil {
		t.Errorf("expected nil sessions, got %v", sessions)
	}
}

func TestList_StatError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission test not meaningful")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")

	parent := filepath.Join(home, ".local", "share")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(parent, "opencode")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	if _, err := (&opencodeSource{}).List(source.ListOptions{}); err == nil {
		t.Fatal("expected stat error, got nil")
	}
}

func TestList_CorruptDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	dir := filepath.Join(home, ".local", "share", "opencode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "opencode.db"), []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := (&opencodeSource{}).List(source.ListOptions{}); err == nil {
		t.Fatal("expected error from corrupt database, got nil")
	}
}

func TestList_HappyPath(t *testing.T) {
	setupV2(t)
	stubRunner(t, false)
	nowFn = func() time.Time { return baseTime }
	t.Cleanup(func() { nowFn = time.Now })

	sessions, err := (&opencodeSource{}).List(source.ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got, want := sessionIDs(sessions), []string{"ses_alpha001", "ses_beta002", "ses_gamma003", "ses_delta004"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", got, want)
	}

	alpha := sessions[0]
	if alpha.Tool != model.ToolOpenCode {
		t.Errorf("Tool = %q, want %q", alpha.Tool, model.ToolOpenCode)
	}
	if alpha.Project != "/Users/foo/prj/demo" {
		t.Errorf("Project = %q", alpha.Project)
	}
	if alpha.Title != "Fix the build" {
		t.Errorf("Title = %q", alpha.Title)
	}
	if alpha.Preview != "Fix the build" {
		t.Errorf("Preview = %q", alpha.Preview)
	}
	if alpha.Model != "prov/model-a" {
		t.Errorf("Model = %q, want prov/model-a", alpha.Model)
	}
	if want := time.UnixMilli(1700000000000).UTC(); !alpha.StartedAt.Equal(want) {
		t.Errorf("StartedAt = %v, want %v", alpha.StartedAt, want)
	}
	if want := time.UnixMilli(1700000600000).UTC(); !alpha.UpdatedAt.Equal(want) {
		t.Errorf("UpdatedAt = %v, want %v", alpha.UpdatedAt, want)
	}
	if alpha.Active {
		t.Error("Active = true, want false when the tool is not running")
	}
	if sessions[1].Model != "" {
		t.Errorf("beta Model = %q, want empty for malformed model JSON", sessions[1].Model)
	}
	if sessions[2].Model != "m2" {
		t.Errorf("gamma Model = %q, want m2", sessions[2].Model)
	}
	if sessions[3].Title != "" || sessions[3].Model != "" || sessions[3].Preview != "" {
		t.Errorf("delta with NULL title/model = %+v", sessions[3])
	}
}

func TestList_Active(t *testing.T) {
	setupV2(t)
	origRunner, origNow := isToolRunning, nowFn
	t.Cleanup(func() { isToolRunning, nowFn = origRunner, origNow })
	isToolRunning = func(string) bool { return true }
	nowFn = func() time.Time { return baseTime }

	sessions, err := (&opencodeSource{}).List(source.ListOptions{Active: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got, want := sessionIDs(sessions), []string{"ses_alpha001"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("active sessions = %v, want %v", got, want)
	}
}

func TestList_Filters(t *testing.T) {
	setupV2(t)
	stubRunner(t, false)

	tests := []struct {
		name string
		opts source.ListOptions
		want []string
	}{
		{
			name: "since",
			opts: source.ListOptions{Since: time.Hour},
			want: []string{"ses_alpha001"},
		},
		{
			name: "date",
			opts: source.ListOptions{OnDate: time.Date(2023, 11, 14, 0, 0, 0, 0, time.UTC)},
			want: []string{"ses_alpha001"},
		},
		{
			name: "project",
			opts: source.ListOptions{Project: "other"},
			want: []string{"ses_beta002", "ses_delta004"},
		},
		{
			name: "exclude project",
			opts: source.ListOptions{ExcludeProjects: []string{"demo"}},
			want: []string{"ses_beta002", "ses_delta004"},
		},
		{
			name: "limit",
			opts: source.ListOptions{Limit: 2},
			want: []string{"ses_alpha001", "ses_beta002"},
		},
		{
			name: "active but tool not running",
			opts: source.ListOptions{Active: true},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nowFn = func() time.Time { return baseTime }
			t.Cleanup(func() { nowFn = time.Now })

			sessions, err := (&opencodeSource{}).List(tt.opts)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if got := sessionIDs(sessions); strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("sessions = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

func TestGet_HomeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	if _, err := (&opencodeSource{}).Get("ses_alpha001"); err == nil {
		t.Fatal("expected error when HOME is empty, got nil")
	}
}

func TestGet_MissingDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")

	sess, err := (&opencodeSource{}).Get("ses_alpha001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess != nil {
		t.Errorf("expected nil session, got %+v", sess)
	}
}

func TestGet_StatError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission test not meaningful")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	parent := filepath.Join(home, ".local", "share")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(parent, "opencode")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	if _, err := (&opencodeSource{}).Get("ses_alpha001"); err == nil {
		t.Fatal("expected stat error, got nil")
	}
}

func TestGet_CorruptDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	dir := filepath.Join(home, ".local", "share", "opencode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "opencode.db"), []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (&opencodeSource{}).Get("ses_alpha001"); err == nil {
		t.Fatal("expected error from corrupt database, got nil")
	}
}

func TestGet_V2(t *testing.T) {
	setupV2(t)
	nowFn = func() time.Time { return baseTime }
	t.Cleanup(func() { nowFn = time.Now })

	sess, err := (&opencodeSource{}).Get("ses_alpha001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess == nil {
		t.Fatal("expected session, got nil")
	}
	if sess.ID != "ses_alpha001" || sess.Title != "Fix the build" {
		t.Errorf("ID/Title = %q/%q", sess.ID, sess.Title)
	}
	if len(sess.Messages) != 3 {
		t.Fatalf("got %d messages, want 3: %+v", len(sess.Messages), sess.Messages)
	}

	first := sess.Messages[0]
	if first.Role != model.RoleUser || first.Content != "Please fix the build" {
		t.Errorf("first message = %+v", first)
	}
	if want := time.UnixMilli(1700000001000).UTC(); !first.Timestamp.Equal(want) {
		t.Errorf("first timestamp = %v, want %v", first.Timestamp, want)
	}

	assistant := sess.Messages[1]
	if assistant.Role != model.RoleAssistant || assistant.Content != "Looking at the build." {
		t.Errorf("assistant = %+v", assistant)
	}
	if len(assistant.ToolCalls) != 3 {
		t.Fatalf("got %d tool calls, want 3", len(assistant.ToolCalls))
	}
	read := assistant.ToolCalls[0]
	if read.ID != "call_1" || read.Name != "read" || read.Output != "package main\n" {
		t.Errorf("read call = %+v", read)
	}
	if read.Input != `{"filePath":"/Users/foo/prj/demo/main.go"}` {
		t.Errorf("read input = %q", read.Input)
	}
	grep := assistant.ToolCalls[1]
	if grep.Output != "main.go:3: TODO" {
		t.Errorf("grep output = %q", grep.Output)
	}
	shell := assistant.ToolCalls[2]
	if !shell.IsError || shell.Output != "exit 1" {
		t.Errorf("shell call = %+v", shell)
	}
	if sess.Messages[2].Content != "Another question about TODO items" {
		t.Errorf("last message = %+v", sess.Messages[2])
	}

	if want := time.UnixMilli(1700000600000).UTC(); !sess.UpdatedAt.Equal(want) {
		t.Errorf("UpdatedAt = %v, want %v", sess.UpdatedAt, want)
	}
}

func TestGet_Prefix(t *testing.T) {
	setupV2(t)
	sess, err := (&opencodeSource{}).Get("ses_alpha")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess == nil || sess.ID != "ses_alpha001" {
		t.Fatalf("prefix match = %+v", sess)
	}
}

func TestGet_AmbiguousPrefix(t *testing.T) {
	setupV2(t)
	_, err := (&opencodeSource{}).Get("ses_")
	if err == nil {
		t.Fatal("expected ambiguity error, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %v, want ambiguous", err)
	}
}

func TestGet_NotFound(t *testing.T) {
	setupV2(t)
	sess, err := (&opencodeSource{}).Get("ses_nope")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess != nil {
		t.Errorf("expected nil session, got %+v", sess)
	}
}

func TestGet_TitleFallback(t *testing.T) {
	setupV2(t)
	sess, err := (&opencodeSource{}).Get("ses_beta002")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess.Title != "beta question" || sess.Preview != "beta question" {
		t.Errorf("title/preview = %q/%q", sess.Title, sess.Preview)
	}
}

func TestGet_NoMessages(t *testing.T) {
	setupV2(t)
	sess, err := (&opencodeSource{}).Get("ses_gamma003")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess.Title != "No messages" {
		t.Errorf("Title = %q", sess.Title)
	}
	if len(sess.Messages) != 0 {
		t.Errorf("expected no messages, got %d", len(sess.Messages))
	}
}

func TestGet_Active(t *testing.T) {
	setupV2(t)
	origRunner, origNow := isToolRunning, nowFn
	t.Cleanup(func() { isToolRunning, nowFn = origRunner, origNow })
	isToolRunning = func(string) bool { return true }
	nowFn = func() time.Time { return baseTime }

	sess, err := (&opencodeSource{}).Get("ses_alpha001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !sess.Active {
		t.Error("Active = false, want true")
	}
}

func TestGet_UpdatedAtFromLastMessage(t *testing.T) {
	setupV2(t)
	sess, err := (&opencodeSource{}).Get("ses_beta002")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if want := time.UnixMilli(1690000002000).UTC(); !sess.UpdatedAt.Equal(want) {
		t.Errorf("UpdatedAt = %v, want last message time %v", sess.UpdatedAt, want)
	}
	if want := time.UnixMilli(1690000001000).UTC(); !sess.StartedAt.Equal(want) {
		t.Errorf("StartedAt = %v, want first message time %v", sess.StartedAt, want)
	}
}

func TestGet_MessageReadError(t *testing.T) {
	setupDB(t, "schema_v2_sessiononly.sql", "")
	if _, err := (&opencodeSource{}).Get("ses_lonely"); err == nil {
		t.Fatal("expected error from missing session_message table, got nil")
	}
}

func TestGet_LegacyMessageReadError(t *testing.T) {
	setupDB(t, "schema_legacy_sessiononly.sql", "")
	if _, err := (&opencodeSource{}).Get("ses_lonely"); err == nil {
		t.Fatal("expected error from missing message table, got nil")
	}
}

func TestGet_LegacyPartsReadError(t *testing.T) {
	setupDB(t, "schema_legacy_noparts.sql", "")
	if _, err := (&opencodeSource{}).Get("ses_lonely"); err == nil {
		t.Fatal("expected error from missing part table, got nil")
	}
}

func TestGet_ResolveError(t *testing.T) {
	// session_v2 exists with wrong column types: readSessionMetas fails inside
	// resolveSession.
	setupDB(t, "schema_v2_badtypes.sql", "")
	if _, err := (&opencodeSource{}).Get("ses_bad"); err == nil {
		t.Fatal("expected scan error from resolveSession, got nil")
	}
}

func TestGet_Legacy(t *testing.T) {
	setupLegacy(t)
	sess, err := (&opencodeSource{}).Get("ses_old001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess == nil {
		t.Fatal("expected session, got nil")
	}
	if sess.Model != "legacy-model" {
		t.Errorf("Model = %q", sess.Model)
	}
	if len(sess.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(sess.Messages))
	}
	assistant := sess.Messages[1]
	if assistant.Content != "legacy answer" {
		t.Errorf("assistant content = %q", assistant.Content)
	}
	if len(assistant.ToolCalls) != 2 {
		t.Fatalf("got %d tool calls, want 2", len(assistant.ToolCalls))
	}
	if assistant.ToolCalls[0].Output != "package legacy" {
		t.Errorf("read output = %q", assistant.ToolCalls[0].Output)
	}
	if !assistant.ToolCalls[1].IsError || assistant.ToolCalls[1].Output != "not found" {
		t.Errorf("error call = %+v", assistant.ToolCalls[1])
	}
}

func TestGet_LegacyMissing(t *testing.T) {
	setupLegacy(t)
	sess, err := (&opencodeSource{}).Get("ses_missing")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess != nil {
		t.Errorf("expected nil session, got %+v", sess)
	}
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------

func TestSearch_HomeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	if _, err := (&opencodeSource{}).Search("x", source.ListOptions{}); err == nil {
		t.Fatal("expected error when HOME is empty, got nil")
	}
}

func TestSearch_V2(t *testing.T) {
	setupV2(t)
	results, err := (&opencodeSource{}).Search("TODO", source.ListOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Session.ID != "ses_alpha001" {
		t.Errorf("session = %s", results[0].Session.ID)
	}
	if len(results[0].Matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(results[0].Matches))
	}
	if results[0].Matches[0].Role != model.RoleUser {
		t.Errorf("role = %q", results[0].Matches[0].Role)
	}
}

func TestSearch_MissingDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")

	results, err := (&opencodeSource{}).Search("x", source.ListOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results, got %v", results)
	}
}

func TestSearch_StatError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, permission test not meaningful")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	parent := filepath.Join(home, ".local", "share")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(parent, "opencode")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	if _, err := (&opencodeSource{}).Search("x", source.ListOptions{}); err == nil {
		t.Fatal("expected stat error, got nil")
	}
}

func TestSearch_CorruptDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	dir := filepath.Join(home, ".local", "share", "opencode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "opencode.db"), []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (&opencodeSource{}).Search("x", source.ListOptions{}); err == nil {
		t.Fatal("expected error from corrupt database, got nil")
	}
}

func TestSearch_MessageReadErrorSkipped(t *testing.T) {
	setupDB(t, "schema_v2_sessiononly.sql", "")
	results, err := (&opencodeSource{}).Search("anything", source.ListOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0 (messages unreadable)", len(results))
	}
}

func TestSearch_NoMatch(t *testing.T) {
	setupV2(t)
	results, err := (&opencodeSource{}).Search("definitely-absent", source.ListOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestSearch_Legacy(t *testing.T) {
	setupLegacy(t)
	results, err := (&opencodeSource{}).Search("legacy answer", source.ListOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].Session.ID != "ses_old001" {
		t.Fatalf("results = %+v", results)
	}
}

// ---------------------------------------------------------------------------
// DB error paths for the readers
// ---------------------------------------------------------------------------

func TestReadSessionMetas_ScanError(t *testing.T) {
	path := setupDB(t, "schema_v2_badtypes.sql", "")
	db := openReadOnly(path)
	defer db.Close()

	if _, err := readSessionMetas(db); err == nil {
		t.Fatal("expected scan error, got nil")
	}
}

func TestReadV2Messages_ScanError(t *testing.T) {
	path := setupDB(t, "schema_v2_badmessage.sql", "")
	db := openReadOnly(path)
	defer db.Close()

	if _, err := readV2Messages(db, "s1"); err == nil {
		t.Fatal("expected scan error, got nil")
	}
}

func TestReadLegacyMessages_ScanError(t *testing.T) {
	path := setupDB(t, "schema_legacy_badtypes.sql", "")
	db := openReadOnly(path)
	defer db.Close()

	if _, err := readLegacyMessages(db, "s1"); err == nil {
		t.Fatal("expected scan error, got nil")
	}
}

func TestReadLegacyParts_ScanError(t *testing.T) {
	path := setupDB(t, "schema_legacy_badparts.sql", "")
	db := openReadOnly(path)
	defer db.Close()

	// The message row scans fine; the part row does not.
	if _, err := readLegacyMessages(db, "s1"); err == nil {
		t.Fatal("expected scan error, got nil")
	}
}

func TestReadV2Messages_MalformedPayload(t *testing.T) {
	path := setupDB(t, "schema_v2.sql", "")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO session_message (id, session_id, type, seq, time_created, time_updated, data)
		VALUES ('m1','s1','user',0,0,0,'not-json')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	ro := openReadOnly(path)
	defer ro.Close()
	messages, err := readV2Messages(ro, "s1")
	if err != nil {
		t.Fatalf("readV2Messages: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("got %d messages, want 0 for malformed payload", len(messages))
	}
}

func TestReadSessionMetas_NoSessionTables(t *testing.T) {
	path := setupDB(t, "schema_empty.sql", "")
	db := openReadOnly(path)
	defer db.Close()

	metas, err := readSessionMetas(db)
	if err != nil {
		t.Fatalf("readSessionMetas: %v", err)
	}
	if metas != nil {
		t.Errorf("expected no sessions, got %v", metas)
	}
}

func TestReadSessionMetas_QueryError(t *testing.T) {
	// The legacy probe succeeds but the query fails: `session` exists with a
	// wrong column set.
	path := setupDB(t, "schema_legacy_badcolumns.sql", "")
	db := openReadOnly(path)
	defer db.Close()

	if _, err := readSessionMetas(db); err == nil {
		t.Fatal("expected query error, got nil")
	}
}

func TestResolveSession_ReadError(t *testing.T) {
	// `session` exists (so the schema probe passes) but the row scan fails.
	path := setupDB(t, "schema_legacy_session_badtypes.sql", "")
	db := openReadOnly(path)
	defer db.Close()

	if _, err := resolveSession(db, "ses_x"); err == nil {
		t.Fatal("expected scan error, got nil")
	}
}

func TestSearch_ListError(t *testing.T) {
	// session_v2 exists (schema detection succeeds) but its rows cannot be
	// scanned, so List fails inside Search.
	setupDB(t, "schema_v2_badtypes.sql", "")
	if _, err := (&opencodeSource{}).Search("x", source.ListOptions{}); err == nil {
		t.Fatal("expected List error, got nil")
	}
}
