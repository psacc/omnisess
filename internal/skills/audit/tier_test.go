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

func TestTier_NamespacedInvocation_matches_bare(t *testing.T) {
	now := time.Now()
	res := Tier(TierInput{
		Skills: []skills.SkillInfo{newSkill("datadog-cli", skills.SourceClaudeGlobal)},
		Invocations: []skills.Invocation{
			newInv("ops:datadog-cli", "user", now),
			newInv("ops:datadog-cli", "user", now),
			newInv("ops:datadog-cli", "user", now),
		},
		Window: 90 * 24 * time.Hour,
		Now:    now,
	})
	if res.Skills[0].Tier != skills.TierKeep {
		t.Errorf("got %v want Keep (namespaced invocations should match bare)", res.Skills[0].Tier)
	}
	if res.Skills[0].UserInvoked != 3 {
		t.Errorf("UserInvoked: got %d want 3", res.Skills[0].UserInvoked)
	}
	if len(res.GhostUsage) != 0 {
		t.Errorf("should not be ghost: %v", res.GhostUsage)
	}
}

func TestTier_NamespacedGhost_keepsOriginalName(t *testing.T) {
	now := time.Now()
	res := Tier(TierInput{
		Skills: []skills.SkillInfo{newSkill("real-skill", skills.SourceClaudeGlobal)},
		Invocations: []skills.Invocation{
			newInv("ops:never-existed", "user", now),
		},
		Window: 90 * 24 * time.Hour,
		Now:    now,
	})
	if len(res.GhostUsage) != 1 || res.GhostUsage[0] != "ops:never-existed" {
		t.Errorf("GhostUsage should record original namespaced name; got %v", res.GhostUsage)
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

func TestTier_MultiTierSort(t *testing.T) {
	// One skill per tier, in reverse order; verify sorted output: Keep < Borderline < Archive < Unknown.
	now := time.Now()
	res := Tier(TierInput{
		Skills: []skills.SkillInfo{
			newSkill("codex-x", skills.SourceCodexProject), // Unknown
			newSkill("unused", skills.SourceClaudeGlobal),  // Archive (0 invocations)
			newSkill("rare", skills.SourceClaudeGlobal),    // Borderline (1 invocation)
			newSkill("popular", skills.SourceClaudeGlobal), // Keep (3 invocations)
		},
		Invocations: []skills.Invocation{
			newInv("popular", "model", now),
			newInv("popular", "user", now),
			newInv("popular", "model", now),
			newInv("rare", "user", now),
		},
		Window: 90 * 24 * time.Hour,
		Now:    now,
	})
	wantTiers := []skills.Tier{skills.TierKeep, skills.TierBorderline, skills.TierArchive, skills.TierUnknown}
	if len(res.Skills) != len(wantTiers) {
		t.Fatalf("got %d skills, want %d", len(res.Skills), len(wantTiers))
	}
	for i, want := range wantTiers {
		if res.Skills[i].Tier != want {
			t.Errorf("position %d: got tier %v want %v (skill: %s)", i, res.Skills[i].Tier, want, res.Skills[i].Skill.Name)
		}
	}
}

func TestTierOrder_BogusValue(t *testing.T) {
	// tierOrder must return 4 (lowest priority, sorts last) for any unknown Tier value.
	got := tierOrder(skills.Tier("Bogus"))
	if got != 4 {
		t.Errorf("tierOrder(Bogus) = %d, want 4", got)
	}
}

func TestTier_SameTierSecondarySort(t *testing.T) {
	// Two Keep-tier skills: "busy" with 5 invocations and "quiet" with 3.
	// They should be sorted by Total descending (busy before quiet) when tiers are equal.
	now := time.Now()
	res := Tier(TierInput{
		Skills: []skills.SkillInfo{
			newSkill("quiet", skills.SourceClaudeGlobal), // 3 invocations → Keep
			newSkill("busy", skills.SourceClaudeGlobal),  // 5 invocations → Keep
		},
		Invocations: []skills.Invocation{
			newInv("quiet", "model", now),
			newInv("quiet", "user", now),
			newInv("quiet", "model", now),
			newInv("busy", "model", now),
			newInv("busy", "user", now),
			newInv("busy", "model", now),
			newInv("busy", "user", now),
			newInv("busy", "model", now),
		},
		Window: 90 * 24 * time.Hour,
		Now:    now,
	})
	if len(res.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(res.Skills))
	}
	if res.Skills[0].Skill.Name != "busy" {
		t.Errorf("expected busy first (higher Total), got %s first", res.Skills[0].Skill.Name)
	}
	if res.Skills[1].Skill.Name != "quiet" {
		t.Errorf("expected quiet second (lower Total), got %s second", res.Skills[1].Skill.Name)
	}
}

func TestTier_LastUsed_PicksLatest(t *testing.T) {
	// Two invocations for same skill; later timestamp should win for LastUsed.
	early := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)
	res := Tier(TierInput{
		Skills: []skills.SkillInfo{newSkill("foo", skills.SourceClaudeGlobal)},
		Invocations: []skills.Invocation{
			newInv("foo", "model", late),
			newInv("foo", "user", early), // earlier — should NOT become LastUsed
		},
		Window: 90 * 24 * time.Hour,
		Now:    now,
	})
	if !res.Skills[0].LastUsed.Equal(late) {
		t.Errorf("LastUsed = %v, want %v (latest invocation)", res.Skills[0].LastUsed, late)
	}
}
