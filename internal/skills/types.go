// Package skills holds the data model for the agent-skill audit feature.
package skills

import "time"

type Source string

const (
	SourceClaudeGlobal  Source = "claude-global"  // ~/.claude/skills/
	SourceAgentsGlobal  Source = "agents-global"  // ~/.agents/skills/
	SourcePlugin        Source = "plugin"         // ~/.claude/plugins/**/skills/
	SourceProjectClaude Source = "project-claude" // <root>/**/.claude/skills/
	SourceProjectAgents Source = "project-agents" // <root>/**/.agents/skills/
	SourceCodexProject  Source = "codex-project"  // <root>/**/.codex/skills/
	SourceCodexGlobal   Source = "codex-global"   // ~/.codex/skills/
)

type SkillInfo struct {
	Name         string // skill name (directory name)
	Path         string // absolute path to SKILL.md
	Source       Source
	DescChars    int       // length of `description` frontmatter field (0 if missing)
	BodyBytes    int64     // size of SKILL.md in bytes
	LastModified time.Time // file mtime
}

type Invocation struct {
	SkillName string
	Timestamp time.Time
	Kind      InvocationKind
	SessionID string
}

type InvocationKind string

const (
	InvocationModel InvocationKind = "model" // tool_use name=Skill, input.skill=X
	InvocationUser  InvocationKind = "user"  // <command-name>/X</command-name>
)

type Tier string

const (
	TierKeep       Tier = "Keep"
	TierBorderline Tier = "Borderline"
	TierArchive    Tier = "Archive"
	TierUnknown    Tier = "Unknown" // telemetry-incomplete (Codex/etc.)
)

type SkillAudit struct {
	Skill        SkillInfo
	Tier         Tier
	ModelInvoked int       // count in window
	UserInvoked  int       // count in window
	Total        int       // = ModelInvoked + UserInvoked
	LastUsed     time.Time // zero value if never used
	Allowlisted  bool
}

type AuditResult struct {
	Window          time.Duration
	GeneratedAt     time.Time
	Roots           []string
	GlobalsIncluded bool
	SessionsParsed  int
	Skills          []SkillAudit // sorted by Tier then descending Total
	GhostUsage      []string     // skill names invoked but not discovered
	UnmatchedAllow  []string     // allowlist entries not matching any skill
	OmnisessVersion string
}
