package cursor

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

type conversationSummary struct {
	ConversationID string
	Title          string
	TLDR           string
	Overview       string
	Model          string
	Mode           string
	UpdatedAt      time.Time
}

// openSQLiteDB opens a SQLite database using the pure-Go driver.
func openSQLiteDB(path string) (*sql.DB, error) {
	return sql.Open("sqlite", path)
}

// readConversationSummaries queries the ai-code-tracking.db for all conversation metadata.
// Returns results ordered by updatedAt descending (most recent first).
// Returns nil, nil if the database file does not exist.
func readConversationSummaries(dbPath string) ([]conversationSummary, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, nil
	}

	// sql.Open with the registered modernc.org/sqlite driver never fails.
	db, _ := openSQLiteDB(dbPath)
	defer db.Close()

	// Check if the table exists before querying.
	var (
		err       error
		tableName string
	)
	err = db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='conversation_summaries'",
	).Scan(&tableName)
	if err != nil {
		// Table does not exist — not an error, just no data.
		return nil, nil
	}

	rows, err := db.Query(`
		SELECT conversationId, title, tldr, overview, model, mode, updatedAt
		FROM conversation_summaries
		ORDER BY updatedAt DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query conversation_summaries: %w", err)
	}
	defer rows.Close()

	var results []conversationSummary
	for rows.Next() {
		var (
			s           conversationSummary
			title       sql.NullString
			tldr        sql.NullString
			overview    sql.NullString
			model       sql.NullString
			mode        sql.NullString
			updatedAtMs sql.NullInt64
		)
		if err := rows.Scan(
			&s.ConversationID,
			&title,
			&tldr,
			&overview,
			&model,
			&mode,
			&updatedAtMs,
		); err != nil {
			return nil, fmt.Errorf("scan conversation_summaries row: %w", err)
		}
		s.Title = title.String
		s.TLDR = tldr.String
		s.Overview = overview.String
		s.Model = model.String
		s.Mode = mode.String
		if updatedAtMs.Valid {
			s.UpdatedAt = time.UnixMilli(updatedAtMs.Int64)
		}
		results = append(results, s)
	}

	return results, nil
}
