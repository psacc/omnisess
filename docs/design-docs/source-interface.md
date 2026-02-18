# Source Interface Contract

**File**: `internal/source/source.go`

## Interface

```go
type Source interface {
    Name() model.Tool
    List(opts ListOptions) ([]model.Session, error)
    Get(sessionID string) (*model.Session, error)
    Search(query string, opts ListOptions) ([]model.SearchResult, error)
}
```

## Method Semantics

### `List(opts)`
- Returns sessions ordered by `UpdatedAt` descending
- `Messages` field is NOT populated (use `Get()` for full content)
- `Preview` is set: first user message truncated to 120 chars, or tool-provided title
- Filters applied: `Since`, `Project` (substring match, case-insensitive), `Active`, `Limit`
- Returns `nil, nil` if no sessions found (not an error)
- Logs warnings to stderr for non-fatal issues (missing files, corrupt entries)

### `Get(sessionID)`
- Returns a single session with full `Messages` populated
- Supports prefix matching: if `sessionID` is 8+ chars, match against full IDs
- Returns error on ambiguous prefix (multiple matches)
- Returns `nil, error` if session not found

### `Search(query, opts)`
- Case-insensitive substring match across message content
- Returns `SearchResult` with `~200 char` snippets centered on match
- Same filters as `List()` apply
- Returns `nil, nil` if no matches (not an error)

## Registration

Sources self-register via `init()`:
```go
func init() {
    source.Register(&mySource{})
}
```

Imported via blank import in `cmd/root.go`:
```go
_ "github.com/psacconier/sessions/internal/source/claude"
```

## Adding a New Source

1. Create package `internal/source/<name>/`
2. Implement `Source` interface
3. Call `source.Register()` in `init()`
4. Add blank import to `cmd/root.go`
5. Add `model.Tool<Name>` constant to `internal/model/session.go`
6. Add to `parseQualifiedID()` switch in `cmd/show.go`
