package index

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"
)

// EnsureSession populates session_metadata + tool_calls rows for the given
// session if the cache is stale or absent.
//
// Cache key: (source_file_mtime, source_file_size, has_full_payloads).
// On a cache miss we rewrite atomically in a single transaction so the
// index is never observed in a half-rewritten state. One transaction per
// session — no global batching across sessions.
func (s *sqliteIndex) EnsureSession(claudeFilePath, conversationID string, includeFullPayloads, rebuild bool, sess *Session) error {
	if sess == nil {
		return errors.New("EnsureSession: nil session")
	}
	if conversationID == "" {
		return errors.New("EnsureSession: empty conversationID")
	}
	st, err := os.Stat(claudeFilePath)
	if err != nil {
		return fmt.Errorf("stat source file: %w", err)
	}
	mtime := st.ModTime().Unix()
	size := st.Size()

	existingMtime, existingSize, existingHasFull, existingPresent, err := s.readCacheKey(conversationID)
	if err != nil {
		return err
	}
	fileChanged := !existingPresent || existingMtime != mtime || existingSize != size
	payloadDowngrade := includeFullPayloads && existingPresent && existingHasFull == 0
	if !fileChanged && !payloadDowngrade && !rebuild {
		// Cache hit, no work.
		return nil
	}
	return s.rewriteSession(claudeFilePath, conversationID, mtime, size, includeFullPayloads, sess)
}

// readCacheKey reads the (mtime, size, has_full_payloads) cache key from
// session_metadata. Returns (_, _, _, false, nil) if the row is absent.
func (s *sqliteIndex) readCacheKey(conversationID string) (mtime, size int64, hasFull int, present bool, err error) {
	row := s.db.QueryRow(
		`SELECT source_file_mtime, source_file_size, has_full_payloads
		 FROM session_metadata WHERE conversation_id = ?`,
		conversationID,
	)
	err = row.Scan(&mtime, &size, &hasFull)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, 0, 0, false, nil
	case err != nil:
		return 0, 0, 0, false, fmt.Errorf("read session_metadata: %w", err)
	}
	return mtime, size, hasFull, true, nil
}

// rewriteSession performs the DELETE→INSERT pair inside one transaction.
func (s *sqliteIndex) rewriteSession(claudeFilePath, conversationID string, mtime, size int64, includeFullPayloads bool, sess *Session) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM tool_calls WHERE conversation_id = ?`, conversationID); err != nil {
		return fmt.Errorf("delete tool_calls: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM session_metadata WHERE conversation_id = ?`, conversationID); err != nil {
		return fmt.Errorf("delete session_metadata: %w", err)
	}

	toolCallCount := len(sess.ToolCalls)
	errorCount := 0
	for _, t := range sess.ToolCalls {
		if t.IsError {
			errorCount++
		}
	}
	hasFull := 0
	if includeFullPayloads {
		hasFull = 1
	}

	if _, err := tx.Exec(
		`INSERT INTO session_metadata (
			conversation_id, provider_name, request_model, response_model,
			started_at, updated_at,
			total_input_tokens, total_output_tokens,
			total_cache_create_tokens, total_cache_read_tokens,
			tool_call_count, error_count,
			source_file_path, source_file_mtime, source_file_size,
			has_full_payloads, indexed_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		conversationID, sess.ProviderName, nullStr(sess.RequestModel), nullStr(sess.ResponseModel),
		unixOrNil(sess.StartedAt), unixOrNil(sess.UpdatedAt),
		sess.TotalInputTokens, sess.TotalOutputTokens,
		sess.TotalCacheCreateTokens, sess.TotalCacheReadTokens,
		toolCallCount, errorCount,
		claudeFilePath, mtime, size,
		hasFull, time.Now().Unix(),
	); err != nil {
		return fmt.Errorf("insert session_metadata: %w", err)
	}

	for _, t := range sess.ToolCalls {
		op := t.OperationName
		if op == "" {
			op = "execute_tool"
		}
		var args, result interface{}
		if includeFullPayloads {
			args = t.ArgumentsJSON
			result = t.ResultJSON
		}
		isErr := 0
		if t.IsError {
			isErr = 1
		}
		// INSERT OR IGNORE: some Claude sessions emit duplicate tool_use_id
		// (observed in long-running sessions with internal retries). The
		// first row wins; later duplicates are silently dropped rather than
		// aborting the whole session re-index.
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO tool_calls (
				conversation_id, tool_call_id, tool_name, tool_type, operation_name, provider_name,
				is_error, ts,
				file_path, file_op, file_lines_added, file_lines_removed, file_content_size,
				arguments_json, result_json
			) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			conversationID, t.ToolCallID, t.ToolName, nullStr(t.ToolType), op, sess.ProviderName,
			isErr, t.Timestamp.Unix(),
			nullStr(t.FilePath), nullStr(t.FileOp),
			nullInt(t.FileLinesAdded, t.FileOp == "edit"),
			nullInt(t.FileLinesRemoved, t.FileOp == "edit"),
			nullInt(t.FileContentSize, t.FileOp == "write"),
			args, result,
		); err != nil {
			return fmt.Errorf("insert tool_calls row %q: %w", t.ToolCallID, err)
		}
	}

	return tx.Commit()
}

// ToolType classifies a tool name per design.md:
//   - "mcp__*" → "mcp"
//   - "Skill" → "skill"
//   - "Agent" or "TaskCreate" → "agent"
//   - everything else → "function"
func ToolType(name string) string {
	switch {
	case len(name) >= 5 && name[:5] == "mcp__":
		return "mcp"
	case name == "Skill":
		return "skill"
	case name == "Agent", name == "TaskCreate":
		return "agent"
	}
	return "function"
}

// nullStr returns nil for empty strings so SQLite stores NULL rather than ”.
func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// nullInt returns the int when applicable=true, else nil (SQL NULL).
func nullInt(v int, applicable bool) interface{} {
	if !applicable {
		return nil
	}
	return v
}

// unixOrNil returns t.Unix() unless t.IsZero(), in which case nil (SQL NULL).
func unixOrNil(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t.Unix()
}
