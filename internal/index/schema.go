package index

import (
	"database/sql"
	"fmt"
)

// schemaSQL is the embedded DDL applied by migrate(). All columns use OTel
// GenAI naming where applicable; file-I/O columns use the omnisess.file.*
// namespace. Multi-statement script; migrate() exec's it once.
const schemaSQL = `
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY
);

INSERT OR IGNORE INTO schema_version (version) VALUES (1);

CREATE TABLE IF NOT EXISTS session_metadata (
    conversation_id            TEXT PRIMARY KEY,
    provider_name              TEXT NOT NULL,
    request_model              TEXT,
    response_model             TEXT,
    started_at                 INTEGER,
    updated_at                 INTEGER,
    total_input_tokens         INTEGER NOT NULL DEFAULT 0,
    total_output_tokens        INTEGER NOT NULL DEFAULT 0,
    total_cache_create_tokens  INTEGER NOT NULL DEFAULT 0,
    total_cache_read_tokens    INTEGER NOT NULL DEFAULT 0,
    tool_call_count            INTEGER NOT NULL DEFAULT 0,
    error_count                INTEGER NOT NULL DEFAULT 0,
    source_file_path           TEXT NOT NULL,
    source_file_mtime          INTEGER NOT NULL,
    source_file_size           INTEGER NOT NULL,
    has_full_payloads          INTEGER NOT NULL DEFAULT 0,
    indexed_at                 INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_session_updated ON session_metadata(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_session_provider ON session_metadata(provider_name);

CREATE TABLE IF NOT EXISTS tool_calls (
    conversation_id   TEXT NOT NULL,
    tool_call_id      TEXT NOT NULL,
    tool_name         TEXT NOT NULL,
    tool_type         TEXT,
    operation_name    TEXT NOT NULL DEFAULT 'execute_tool',
    provider_name     TEXT NOT NULL,
    is_error          INTEGER NOT NULL DEFAULT 0,
    ts                INTEGER NOT NULL,
    file_path         TEXT,
    file_op           TEXT,
    file_lines_added  INTEGER,
    file_lines_removed INTEGER,
    file_content_size INTEGER,
    arguments_json    TEXT,
    result_json       TEXT,
    PRIMARY KEY (conversation_id, tool_call_id),
    FOREIGN KEY (conversation_id) REFERENCES session_metadata(conversation_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_tool_calls_conv ON tool_calls(conversation_id);
CREATE INDEX IF NOT EXISTS idx_tool_calls_name ON tool_calls(tool_name);
CREATE INDEX IF NOT EXISTS idx_tool_calls_file ON tool_calls(file_path) WHERE file_path IS NOT NULL;
`

// migrate applies the schema. Safe to call multiple times — IF NOT EXISTS
// guards every statement and INSERT OR IGNORE makes the version-row write
// idempotent. schemaSQL is a single multi-statement script so this function
// has a single error path.
func migrate(db *sql.DB) error {
	if _, err := db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return nil
}
