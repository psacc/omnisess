# Gemini — Local Data Format (Reference for Later)

## Paths

- **Conversations**: `~/.gemini/antigravity/conversations/*.pb` (encrypted protobuf — NOT parseable)
- **History markers**: `~/.gemini/history/<project-name>/`
- **User settings**: `~/.gemini/antigravity/user_settings.pb`
- **Project map**: `~/.gemini/projects.json`

## Known Limitations

Conversation `.pb` files are **encrypted**. No readable strings, no protobuf field tags.
Without Google publishing the schema or adding an export feature, direct parsing is not viable.

## CLI Fallback

```bash
gemini --list-sessions          # List sessions for current project
gemini --resume latest          # Resume most recent
gemini --resume <index>         # Resume by index
```

The `--list-sessions` output can be parsed for session listing, but provides no message content.

## Status: DEFERRED

Implementation deferred. Stub source returns empty results.
Only viable path is CLI output parsing — limited to listing, no content search.
