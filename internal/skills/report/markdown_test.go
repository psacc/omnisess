package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/psacc/omnisess/internal/skills"
)

func sampleResult() skills.AuditResult {
	now := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	return skills.AuditResult{
		Window:          90 * 24 * time.Hour,
		GeneratedAt:     now,
		Roots:           []string{"/home/u/repo-a"},
		GlobalsIncluded: true,
		SessionsParsed:  42,
		OmnisessVersion: "0.2.0",
		Skills: []skills.SkillAudit{
			{Skill: skills.SkillInfo{Name: "frequent", Source: skills.SourceClaudeGlobal, DescChars: 80}, Tier: skills.TierKeep, ModelInvoked: 5, UserInvoked: 7, Total: 12, LastUsed: now.Add(-24 * time.Hour)},
			{Skill: skills.SkillInfo{Name: "rare", Source: skills.SourceProjectClaude, DescChars: 200}, Tier: skills.TierBorderline, ModelInvoked: 1, UserInvoked: 1, Total: 2, LastUsed: now.Add(-50 * 24 * time.Hour)},
			{Skill: skills.SkillInfo{Name: "unused-big", Source: skills.SourceClaudeGlobal, DescChars: 500}, Tier: skills.TierArchive, Total: 0},
			{Skill: skills.SkillInfo{Name: "unused-small", Source: skills.SourceClaudeGlobal, DescChars: 60}, Tier: skills.TierArchive, Total: 0},
			{Skill: skills.SkillInfo{Name: "codex-only", Source: skills.SourceCodexProject, DescChars: 100}, Tier: skills.TierUnknown},
		},
		GhostUsage:     []string{"phantom"},
		UnmatchedAllow: []string{"never-existed"},
	}
}

func TestMarkdownContainsSections(t *testing.T) {
	var buf bytes.Buffer
	if err := Markdown(&buf, sampleResult()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"# Skills Audit",
		"## Summary",
		"## Top by usage",
		"## Archive candidates",
		"## Borderline",
		"## Unknown",
		"## Ghost usage",
		"## Unmatched allowlist",
		"## Provenance",
		"frequent",
		"unused-big",
		"rare",
		"codex-only",
		"phantom",
		"never-existed",
		"0.2.0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestMarkdownArchiveSortedByCostDesc(t *testing.T) {
	var buf bytes.Buffer
	Markdown(&buf, sampleResult())
	out := buf.String()
	iBig := strings.Index(out, "unused-big")
	iSmall := strings.Index(out, "unused-small")
	if iBig < 0 || iSmall < 0 {
		t.Fatal("expected both archive entries in output")
	}
	if iBig > iSmall {
		t.Errorf("unused-big (DescChars=500) should appear before unused-small (DescChars=60) in archive section")
	}
}

func TestMarkdownDeterministic(t *testing.T) {
	var a, b bytes.Buffer
	res := sampleResult()
	Markdown(&a, res)
	Markdown(&b, res)
	if a.String() != b.String() {
		t.Errorf("markdown output should be deterministic; diff:\n--A--\n%s\n--B--\n%s", a.String(), b.String())
	}
}
