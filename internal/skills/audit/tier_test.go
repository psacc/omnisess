package audit

import (
	"testing"
	"time"

	"github.com/psacc/omnisess/internal/skills"
)

func newSkill(name string, src skills.Source) skills.SkillInfo {
	return skills.SkillInfo{Name: name, Source: src, DescChars: 100, BodyBytes: 1000, LastModified: time.Now()}
}

func newInv(name, kind string, t time.Time) skills.Invocation {
	return skills.Invocation{SkillName: name, Timestamp: t, Kind: skills.InvocationKind(kind)}
}

func TestTier_Keep_3OrMore(t *testing.T) {
	now := time.Now()
	inv := []skills.Invocation{
		newInv("foo", "model", now),
		newInv("foo", "user", now),
		newInv("foo", "model", now),
	}
	res := Tier(TierInput{
		Skills:      []skills.SkillInfo{newSkill("foo", skills.SourceClaudeGlobal)},
		Invocations: inv,
		Window:      90 * 24 * time.Hour,
		Now:         now,
	})
	if res.Skills[0].Tier != skills.TierKeep {
		t.Errorf("got %v want Keep", res.Skills[0].Tier)
	}
	if res.Skills[0].Total != 3 {
		t.Errorf("Total: got %d want 3", res.Skills[0].Total)
	}
}

func TestTier_Borderline_1or2(t *testing.T) {
	now := time.Now()
	res := Tier(TierInput{
		Skills:      []skills.SkillInfo{newSkill("foo", skills.SourceClaudeGlobal)},
		Invocations: []skills.Invocation{newInv("foo", "model", now)},
		Window:      90 * 24 * time.Hour,
		Now:         now,
	})
	if res.Skills[0].Tier != skills.TierBorderline {
		t.Errorf("got %v want Borderline", res.Skills[0].Tier)
	}
}

func TestTier_Archive_zero(t *testing.T) {
	now := time.Now()
	res := Tier(TierInput{
		Skills:      []skills.SkillInfo{newSkill("foo", skills.SourceClaudeGlobal)},
		Invocations: nil,
		Window:      90 * 24 * time.Hour,
		Now:         now,
	})
	if res.Skills[0].Tier != skills.TierArchive {
		t.Errorf("got %v want Archive", res.Skills[0].Tier)
	}
}

func TestTier_Allowlist_overrides_archive(t *testing.T) {
	now := time.Now()
	res := Tier(TierInput{
		Skills:      []skills.SkillInfo{newSkill("foo", skills.SourceClaudeGlobal)},
		Invocations: nil,
		Allowlist:   []string{"foo"},
		Window:      90 * 24 * time.Hour,
		Now:         now,
	})
	if res.Skills[0].Tier != skills.TierKeep {
		t.Errorf("got %v want Keep (allowlisted)", res.Skills[0].Tier)
	}
	if !res.Skills[0].Allowlisted {
		t.Error("Allowlisted should be true")
	}
}

func TestTier_Codex_forced_unknown(t *testing.T) {
	now := time.Now()
	res := Tier(TierInput{
		Skills:      []skills.SkillInfo{newSkill("foo", skills.SourceCodexProject)},
		Invocations: nil,
		Window:      90 * 24 * time.Hour,
		Now:         now,
	})
	if res.Skills[0].Tier != skills.TierUnknown {
		t.Errorf("got %v want Unknown for codex source", res.Skills[0].Tier)
	}
}

func TestTier_OutsideWindow_notCounted(t *testing.T) {
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	old := now.Add(-200 * 24 * time.Hour)
	res := Tier(TierInput{
		Skills:      []skills.SkillInfo{newSkill("foo", skills.SourceClaudeGlobal)},
		Invocations: []skills.Invocation{newInv("foo", "model", old)},
		Window:      90 * 24 * time.Hour,
		Now:         now,
	})
	if res.Skills[0].Tier != skills.TierArchive {
		t.Errorf("got %v want Archive (old invocation outside window)", res.Skills[0].Tier)
	}
}

func TestTier_GhostUsage_recorded(t *testing.T) {
	now := time.Now()
	res := Tier(TierInput{
		Skills:      []skills.SkillInfo{newSkill("foo", skills.SourceClaudeGlobal)},
		Invocations: []skills.Invocation{newInv("missing-skill", "model", now)},
		Window:      90 * 24 * time.Hour,
		Now:         now,
	})
	found := false
	for _, g := range res.GhostUsage {
		if g == "missing-skill" {
			found = true
		}
	}
	if !found {
		t.Errorf("missing-skill should appear in GhostUsage; got %v", res.GhostUsage)
	}
}

func TestTier_UnmatchedAllowlist_recorded(t *testing.T) {
	now := time.Now()
	res := Tier(TierInput{
		Skills:    []skills.SkillInfo{newSkill("foo", skills.SourceClaudeGlobal)},
		Allowlist: []string{"foo", "ghost-allow"},
		Window:    90 * 24 * time.Hour,
		Now:       now,
	})
	if len(res.UnmatchedAllow) != 1 || res.UnmatchedAllow[0] != "ghost-allow" {
		t.Errorf("UnmatchedAllow: got %v want [ghost-allow]", res.UnmatchedAllow)
	}
}
