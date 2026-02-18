# Cursor — Local Data Format

## Paths

- **Chat stores**: `~/.cursor/chats/<workspace-id>/<agent-id>/store.db` (SQLite)
- **AI tracking DB**: `~/.cursor/ai-tracking/ai-code-tracking.db` (SQLite)
- **Agent transcripts**: `~/.cursor/projects/<project-dashes>/agent-transcripts/<agent-id>.txt`
- **Prompt history**: `~/.cursor/prompt_history.json`

## Workspace ID Mapping

Workspace ID = MD5 hash of absolute project path:
`MD5("/Users/paolo/prj/finn/b2b-orders-api")` = `6a4ed208d874bc31c17fb549de8edded`

## Project Name Encoding (in projects/ dir)

Similar to Claude but WITHOUT leading dash:
`/Users/paolo/prj/foo` → `Users-paolo-prj-foo`

## ai-code-tracking.db Schema

### conversation_summaries table
```sql
CREATE TABLE conversation_summaries (
    conversationId TEXT PRIMARY KEY,
    title TEXT,
    tldr TEXT,
    overview TEXT,
    summaryBullets TEXT,
    model TEXT,
    mode TEXT,
    updatedAt INTEGER  -- Unix epoch milliseconds
);
```

This is the richest metadata source for Cursor sessions.

## Chat store.db Schema

### meta table
Key `0` contains hex-encoded JSON:
```json
{
    "agentId": "37c863b6-...",
    "name": "Worktree CLI Setup",
    "mode": "default",
    "createdAt": 1759767571411,
    "lastUsedModel": "default"
}
```

### blobs table
Binary blobs keyed by ID. Contains actual conversation content but format is opaque/serialized.

## Agent Transcript Format

Plain text file. Sections delimited by role markers:
```
user:
What are the modified or untracked files?

assistant:
I'll check the git status for you...

[Tool call: Bash]
git status

[Tool result]
On branch main...
```

## Prompt History (prompt_history.json)

Simple JSON array of strings — most recent prompts:
```json
["what are the modified files?", "rm the worktree", "create a new worktree"]
```
