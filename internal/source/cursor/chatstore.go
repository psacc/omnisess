package cursor

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type chatMeta struct {
	AgentID   string `json:"agentId"`
	Name      string `json:"name"`
	Mode      string `json:"mode"`
	CreatedAt int64  `json:"createdAt"` // unix milliseconds
	Model     string `json:"lastUsedModel"`
}

// readAllChatMeta scans ~/.cursor/chats/<workspace>/<agent>/store.db files
// and reads their hex-encoded JSON metadata.
// Returns a map from agentId to chatMeta.
func readAllChatMeta(homeDir string) map[string]chatMeta {
	result := make(map[string]chatMeta)

	chatsDir := filepath.Join(homeDir, ".cursor", "chats")
	workspaces, err := os.ReadDir(chatsDir)
	if err != nil {
		return result
	}

	for _, ws := range workspaces {
		if !ws.IsDir() {
			continue
		}
		agents, err := os.ReadDir(filepath.Join(chatsDir, ws.Name()))
		if err != nil {
			continue
		}
		for _, agent := range agents {
			if !agent.IsDir() {
				continue
			}
			dbPath := filepath.Join(chatsDir, ws.Name(), agent.Name(), "store.db")
			meta, err := readChatStoreMeta(dbPath)
			if err != nil {
				continue
			}
			result[meta.AgentID] = meta
		}
	}

	return result
}

// readChatStoreMeta reads the meta key "0" from a Cursor chat store.db.
// The value is hex-encoded JSON.
func readChatStoreMeta(dbPath string) (chatMeta, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return chatMeta{}, fmt.Errorf("store.db not found")
	}

	db, err := openSQLiteDB(dbPath)
	if err != nil {
		return chatMeta{}, err
	}
	defer db.Close()

	var hexValue string
	err = db.QueryRow("SELECT value FROM meta WHERE key='0'").Scan(&hexValue)
	if err != nil {
		return chatMeta{}, fmt.Errorf("query meta: %w", err)
	}

	jsonBytes, err := hex.DecodeString(hexValue)
	if err != nil {
		return chatMeta{}, fmt.Errorf("decode hex: %w", err)
	}

	var meta chatMeta
	if err := json.Unmarshal(jsonBytes, &meta); err != nil {
		return chatMeta{}, fmt.Errorf("unmarshal meta: %w", err)
	}

	return meta, nil
}

// chatMetaCreatedAt converts the unix millisecond timestamp to time.Time.
func chatMetaCreatedAt(m chatMeta) time.Time {
	if m.CreatedAt == 0 {
		return time.Time{}
	}
	return time.UnixMilli(m.CreatedAt)
}
