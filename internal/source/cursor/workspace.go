package cursor

import (
	"os"
	"path/filepath"
	"strings"
)

// projectPathFromDir decodes a Cursor project directory name back to an absolute path.
// Cursor encodes "/Users/paolo.sacconier/prj/foo" as "Users-paolo-sacconier-prj-foo"
// (no leading dash, all path separators and dots become dashes).
//
// Strategy: use the known home directory as anchor, then walk the filesystem
// greedily matching the longest directory name at each level.
func projectPathFromDir(dirName string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "/" + strings.ReplaceAll(dirName, "-", "/")
	}

	// Encode the home directory the same way Cursor does.
	homeEncoded := encodePath(homeDir)

	if strings.HasPrefix(dirName, homeEncoded) {
		suffix := strings.TrimPrefix(dirName, homeEncoded)
		suffix = strings.TrimPrefix(suffix, "-")
		if suffix == "" {
			return homeDir
		}
		resolved := resolvePathGreedy(homeDir, suffix)
		if resolved != "" {
			return resolved
		}
	}

	// Fallback: naive decode.
	return "/" + strings.ReplaceAll(dirName, "-", "/")
}

// encodePath encodes a path the way Cursor does: replace / and . with -, strip leading /.
func encodePath(p string) string {
	encoded := strings.TrimPrefix(p, "/")
	encoded = strings.ReplaceAll(encoded, "/", "-")
	encoded = strings.ReplaceAll(encoded, ".", "-")
	return encoded
}

// resolvePathGreedy walks the filesystem greedily to decode an encoded suffix.
// At each directory level, it tries to match the longest possible directory name
// from the remaining encoded string.
//
// For "prj-finn-b2b-orders-api", starting at /Users/paolo.sacconier:
// 1. List children of /Users/paolo.sacconier → find "prj" matches prefix
// 2. List children of /Users/paolo.sacconier/prj → find "finn" matches
// 3. List children of .../prj/finn → find "b2b-orders-api" matches (longest)
func resolvePathGreedy(base string, encoded string) string {
	if encoded == "" {
		return base
	}

	// List children of base directory.
	entries, err := os.ReadDir(base)
	if err != nil {
		return ""
	}

	// Try longest match first: for each child directory, encode its name
	// and check if the encoded suffix starts with it.
	type match struct {
		path      string
		remaining string
	}
	var matches []match

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		childEncoded := encodePath(e.Name())
		if encoded == childEncoded {
			// Exact full match — this is the directory.
			return filepath.Join(base, e.Name())
		}
		if strings.HasPrefix(encoded, childEncoded+"-") {
			remaining := strings.TrimPrefix(encoded, childEncoded+"-")
			matches = append(matches, match{
				path:      filepath.Join(base, e.Name()),
				remaining: remaining,
			})
		}
	}

	// Sort by match length descending (longest encoded match first).
	// This ensures "b2b-orders-api" matches before "b2b".
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			if len(matches[j].remaining) < len(matches[i].remaining) {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	// Try each match, longest first.
	for _, m := range matches {
		result := resolvePathGreedy(m.path, m.remaining)
		if result != "" {
			return result
		}
	}

	return ""
}

// findTranscriptFile scans ~/.cursor/projects/*/agent-transcripts/ for a file
// matching conversationID.txt.
func findTranscriptFile(homeDir string, conversationID string) (projectPath string, transcriptPath string) {
	projectsDir := filepath.Join(homeDir, ".cursor", "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return "", ""
	}

	fileName := conversationID + ".txt"

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(projectsDir, entry.Name(), "agent-transcripts", fileName)
		if _, err := os.Stat(candidate); err == nil {
			return projectPathFromDir(entry.Name()), candidate
		}
	}

	return "", ""
}

type transcriptEntry struct {
	ProjectDirName string
	ProjectPath    string
	ConversationID string
	FilePath       string
}

func listAllTranscripts(homeDir string) []transcriptEntry {
	projectsDir := filepath.Join(homeDir, ".cursor", "projects")
	projectEntries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil
	}

	var results []transcriptEntry
	for _, pEntry := range projectEntries {
		if !pEntry.IsDir() {
			continue
		}
		transcriptsDir := filepath.Join(projectsDir, pEntry.Name(), "agent-transcripts")
		files, err := os.ReadDir(transcriptsDir)
		if err != nil {
			continue
		}
		projPath := projectPathFromDir(pEntry.Name())
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".txt") {
				continue
			}
			convID := strings.TrimSuffix(f.Name(), ".txt")
			results = append(results, transcriptEntry{
				ProjectDirName: pEntry.Name(),
				ProjectPath:    projPath,
				ConversationID: convID,
				FilePath:       filepath.Join(transcriptsDir, f.Name()),
			})
		}
	}

	return results
}
