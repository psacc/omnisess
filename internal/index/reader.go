package index

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"
)

// QuerySession returns the SessionStats for one conversation, or nil if not present.
func (s *sqliteIndex) QuerySession(conversationID string) (*SessionStats, error) {
	out := &SessionStats{ConversationID: conversationID}

	var (
		startedAt, updatedAt sql.NullInt64
		reqModel, respModel  sql.NullString
		hasFull              int
	)
	err := s.db.QueryRow(
		`SELECT provider_name, request_model, response_model,
		        started_at, updated_at,
		        total_input_tokens, total_output_tokens,
		        total_cache_create_tokens, total_cache_read_tokens,
		        tool_call_count, error_count, has_full_payloads
		   FROM session_metadata WHERE conversation_id = ?`,
		conversationID,
	).Scan(
		&out.ProviderName, &reqModel, &respModel,
		&startedAt, &updatedAt,
		&out.TotalInputTokens, &out.TotalOutputTokens,
		&out.TotalCacheCreateTokens, &out.TotalCacheReadTokens,
		&out.ToolCallCount, &out.ErrorCount, &hasFull,
	)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("query session_metadata: %w", err)
	}
	if reqModel.Valid {
		out.RequestModel = reqModel.String
	}
	if respModel.Valid {
		out.ResponseModel = respModel.String
	}
	if startedAt.Valid {
		out.StartedAt = time.Unix(startedAt.Int64, 0).UTC()
	}
	if updatedAt.Valid {
		out.UpdatedAt = time.Unix(updatedAt.Int64, 0).UTC()
	}
	out.HasFullPayloads = hasFull == 1

	tools, err := s.aggregateTools(
		`SELECT tool_name, COALESCE(tool_type,'') AS tool_type,
		        COUNT(*) AS cnt, SUM(is_error) AS errs
		 FROM tool_calls WHERE conversation_id = ?
		 GROUP BY tool_name, tool_type
		 ORDER BY cnt DESC, tool_name ASC`,
		conversationID,
	)
	if err != nil {
		return nil, err
	}
	out.Tools = tools

	files, err := s.aggregateFiles(
		`SELECT file_path, file_op,
		        COALESCE(file_lines_added, 0) AS adds,
		        COALESCE(file_lines_removed, 0) AS rems
		 FROM tool_calls
		 WHERE conversation_id = ? AND file_path IS NOT NULL AND file_path <> ''`,
		conversationID,
	)
	if err != nil {
		return nil, err
	}
	out.Files = files

	return out, nil
}

// QueryWindow returns aggregate stats across sessions whose updated_at falls
// in [start, end). providerName="" disables the provider filter.
func (s *sqliteIndex) QueryWindow(start, end time.Time, providerName string) (*AggregateStats, error) {
	out := &AggregateStats{WindowStart: start, WindowEnd: end}

	args := []interface{}{start.Unix(), end.Unix()}
	provFilter := ""
	if providerName != "" {
		provFilter = " AND provider_name = ?"
		args = append(args, providerName)
	}

	err := s.db.QueryRow(
		`SELECT COUNT(*) AS sessions,
		        COALESCE(SUM(total_input_tokens),0),
		        COALESCE(SUM(total_output_tokens),0),
		        COALESCE(SUM(total_cache_create_tokens),0),
		        COALESCE(SUM(total_cache_read_tokens),0),
		        COALESCE(SUM(tool_call_count),0),
		        COALESCE(SUM(error_count),0)
		 FROM session_metadata
		 WHERE updated_at >= ? AND updated_at < ?`+provFilter,
		args...,
	).Scan(
		&out.SessionCount,
		&out.TotalInputTokens, &out.TotalOutputTokens,
		&out.TotalCacheCreateTokens, &out.TotalCacheReadTokens,
		&out.ToolCallCount, &out.ErrorCount,
	)
	if err != nil {
		return nil, fmt.Errorf("query window aggregates: %w", err)
	}

	tcProv := ""
	tcArgs := []interface{}{start.Unix(), end.Unix()}
	if providerName != "" {
		tcProv = " AND tc.provider_name = ?"
		tcArgs = append(tcArgs, providerName)
	}
	tools, err := s.aggregateTools(
		`SELECT tc.tool_name, COALESCE(tc.tool_type,'') AS tool_type,
		        COUNT(*) AS cnt, SUM(tc.is_error) AS errs
		 FROM tool_calls tc
		 JOIN session_metadata sm ON sm.conversation_id = tc.conversation_id
		 WHERE sm.updated_at >= ? AND sm.updated_at < ?`+tcProv+`
		 GROUP BY tc.tool_name, tc.tool_type
		 ORDER BY cnt DESC, tc.tool_name ASC`,
		tcArgs...,
	)
	if err != nil {
		return nil, err
	}
	out.Tools = tools

	files, err := s.aggregateFiles(
		`SELECT tc.file_path, tc.file_op,
		        COALESCE(tc.file_lines_added, 0),
		        COALESCE(tc.file_lines_removed, 0)
		 FROM tool_calls tc
		 JOIN session_metadata sm ON sm.conversation_id = tc.conversation_id
		 WHERE sm.updated_at >= ? AND sm.updated_at < ?`+tcProv+`
		   AND tc.file_path IS NOT NULL AND tc.file_path <> ''`,
		tcArgs...,
	)
	if err != nil {
		return nil, err
	}
	out.Files = files

	return out, nil
}

// aggregateTools runs a query that returns rows of
// (tool_name, tool_type, count, sum_is_error) and returns a sorted slice.
func (s *sqliteIndex) aggregateTools(query string, args ...interface{}) ([]ToolCount, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query tools: %w", err)
	}
	defer rows.Close()
	var out []ToolCount
	for rows.Next() {
		var tc ToolCount
		var errs sql.NullInt64
		if err := rows.Scan(&tc.Name, &tc.Type, &tc.Count, &errs); err != nil {
			return nil, fmt.Errorf("scan tool count: %w", err)
		}
		if errs.Valid {
			tc.ErrorCount = int(errs.Int64)
		}
		out = append(out, tc)
	}
	return out, rows.Err()
}

// aggregateFiles runs a query that returns rows of (file_path, file_op,
// lines_added, lines_removed) and aggregates them by path into FileActivity.
func (s *sqliteIndex) aggregateFiles(query string, args ...interface{}) ([]FileActivity, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query files: %w", err)
	}
	defer rows.Close()
	agg := make(map[string]*FileActivity)
	for rows.Next() {
		var path, op string
		var adds, rems int
		if err := rows.Scan(&path, &op, &adds, &rems); err != nil {
			return nil, fmt.Errorf("scan file row: %w", err)
		}
		a, ok := agg[path]
		if !ok {
			a = &FileActivity{Path: path}
			agg[path] = a
		}
		switch op {
		case "read":
			a.Reads++
		case "write":
			a.Writes++
		case "edit":
			a.Edits++
		}
		a.LinesAdded += adds
		a.LinesRemoved += rems
	}
	out := make([]FileActivity, 0, len(agg))
	for _, a := range agg {
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool {
		oi := out[i].Reads + out[i].Writes + out[i].Edits
		oj := out[j].Reads + out[j].Writes + out[j].Edits
		if oi != oj {
			return oi > oj
		}
		return out[i].Path < out[j].Path
	})
	return out, rows.Err()
}
