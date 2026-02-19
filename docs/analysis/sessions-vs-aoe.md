# sessions vs Agents of Empire (AoE)

## Date

2026-02-19

## Relationship

**No, sessions is not a replacement for AoE.** They are complementary tools occupying different niches. `sessions` is a **read-only observability layer** -- it discovers, searches, and displays existing AI coding sessions across tools (Claude, Cursor, Codex, Gemini). AoE is an **operational control plane** -- it creates, runs, and manages AI agent sessions in parallel via tmux. The overlap is minimal: both list sessions and have a TUI, but `sessions` discovers sessions it did not create while AoE only manages sessions it spawned. A developer would reasonably use both: AoE to orchestrate agents, `sessions` to search and analyze what happened across all tools afterward.

## Our Unique Advantages

- **Full-text search across message content** -- AoE has zero content search; it can only filter by session name/path
- **Session history viewer** (`show` command) -- AoE can attach to live tmux sessions but cannot display past conversation transcripts
- **Deep Cursor integration** -- merges 3 Cursor data sources (SQLite + transcripts + chat store); AoE does not support Cursor at all
- **Read-only safety invariant** -- cannot corrupt source data; safe to run against active sessions
- **JSON scripting output** on all commands with sanitized control characters -- better automation/pipeline story
- **Clean Source interface** -- adding a new tool is an isolated package with zero cross-source coupling
- **Pure Go, no CGO** -- simpler cross-compilation than Rust + libgit2 + vendored OpenSSL

## AoE's Unique Advantages

- **Session lifecycle management** -- create, run, persist, and delete sessions (we are read-only by design)
- **tmux-native persistence** -- sessions survive TUI close, terminal close, SSH disconnect
- **Advanced TUI** -- dialogs, settings editor, diff view, live preview, mouse support, fuzzy search
- **Live per-agent status** via tmux pane content scraping (Running/Waiting/Idle/Error)
- **Git worktree-first workflow** -- branch + worktree + session in one command
- **Docker + Apple Container sandboxing**
- **5 working AI tool integrations** (Claude, OpenCode, Mistral Vibe, Codex, Gemini) vs our 2 real + 2 stubs
- **Per-repo config** (`.aoe/config.toml`) with cryptographic hook trust
- **Session grouping, profiles, custom instruction injection**

## Integration vs Feature Parity

**Do not chase feature parity. Double down on our niche: retrospective session intelligence.** AoE's strength is real-time orchestration -- replicating tmux lifecycle management, Docker sandboxing, or worktree integration would be massive effort (L each) that contradicts our read-only philosophy. Instead, own the "what happened across all my AI sessions" question that AoE cannot answer. The only AoE features worth borrowing are small UX improvements (TUI search/filter) that make our existing value more accessible. Long-term, integration is a possibility (AoE manages sessions, `sessions` analyzes them), but there is no urgent need -- the tools already coexist naturally since we read the same files AoE's agents produce.

## Actionable Next Steps

| Rank | Feature | Why | Size |
|------|---------|-----|------|
| 1 | Implement Codex + Gemini sources for real | Closes credibility gap -- we claim 4 tools, deliver 2. Source interface makes this isolated. | M |
| 2 | Surface cost/token data from Claude JSONL | Neither tool does this. Data is already on disk. Immediate value for engineers managing AI spend. | S |
| 3 | TUI search/filter (`/` shortcut) | With 50+ sessions, scrolling is painful. Bubbletea textinput component is ready. | S |
| 4 | Session recap/summary command | Our unique differentiator territory. AoE shows live sessions but cannot tell you what you worked on yesterday. Exec plans 004/006 already designed. | M |
| 5 | Linux active detection | AoE supports macOS + Linux. We are macOS-only. Low effort to unblock a meaningful user segment. | S |
