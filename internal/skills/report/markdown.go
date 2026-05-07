// Package report renders AuditResult as markdown or JSON.
package report

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/psacc/omnisess/internal/skills"
)

// Markdown writes the audit result as a markdown report.
func Markdown(w io.Writer, r skills.AuditResult) error {
	f := func(format string, args ...interface{}) {
		fmt.Fprintf(w, format, args...)
	}

	days := int(r.Window.Hours() / 24)
	f("# Skills Audit\n\n")
	f("Window: last %d days · Generated: %s\n\n", days, r.GeneratedAt.UTC().Format(time.RFC3339))

	// Summary
	var nKeep, nBorder, nArch, nUnk int
	var archiveDescChars int
	for _, s := range r.Skills {
		switch s.Tier {
		case skills.TierKeep:
			nKeep++
		case skills.TierBorderline:
			nBorder++
		case skills.TierArchive:
			nArch++
			archiveDescChars += s.Skill.DescChars
		case skills.TierUnknown:
			nUnk++
		}
	}
	f("## Summary\n\n")
	f("| Tier | Count |\n|---|---|\n")
	f("| Keep | %d |\n| Borderline | %d |\n| Archive | %d |\n| Unknown | %d |\n", nKeep, nBorder, nArch, nUnk)
	f("\nEstimated tokens reclaimable if all archive candidates removed: ~%d (≈ desc chars / 4)\n\n", archiveDescChars/4)

	// Top by usage
	f("## Top by usage\n\n")
	top := append([]skills.SkillAudit(nil), r.Skills...)
	sort.SliceStable(top, func(i, j int) bool { return top[i].Total > top[j].Total })
	f("| Skill | Source | Total | Model | User | Last used |\n|---|---|---|---|---|---|\n")
	for i, s := range top {
		if i >= 20 || s.Total == 0 {
			break
		}
		f("| `%s` | %s | %d | %d | %d | %s |\n", s.Skill.Name, s.Skill.Source, s.Total, s.ModelInvoked, s.UserInvoked, fmtTime(s.LastUsed))
	}
	f("\n")

	// Archive candidates (sorted by DescChars descending, biggest wins first)
	archive := filterTier(r.Skills, skills.TierArchive)
	sort.SliceStable(archive, func(i, j int) bool {
		return archive[i].Skill.DescChars > archive[j].Skill.DescChars
	})
	f("## Archive candidates\n\n")
	if len(archive) == 0 {
		f("_None._\n\n")
	} else {
		f("Sorted by description size (biggest token wins first).\n\n")
		f("| Skill | Source | DescChars | BodyBytes | Last modified | Path |\n|---|---|---|---|---|---|\n")
		for _, s := range archive {
			f("| `%s` | %s | %d | %d | %s | `%s` |\n",
				s.Skill.Name, s.Skill.Source, s.Skill.DescChars, s.Skill.BodyBytes,
				fmtTime(s.Skill.LastModified), s.Skill.Path)
		}
		f("\n")
	}

	// Borderline
	border := filterTier(r.Skills, skills.TierBorderline)
	f("## Borderline\n\n")
	if len(border) == 0 {
		f("_None._\n\n")
	} else {
		f("| Skill | Source | Total | Last used |\n|---|---|---|---|\n")
		for _, s := range border {
			f("| `%s` | %s | %d | %s |\n", s.Skill.Name, s.Skill.Source, s.Total, fmtTime(s.LastUsed))
		}
		f("\n")
	}

	// Unknown
	unk := filterTier(r.Skills, skills.TierUnknown)
	f("## Unknown\n\n")
	if len(unk) == 0 {
		f("_None._\n\n")
	} else {
		f("Telemetry-incomplete sources (Codex/etc.); not parsed in v0.\n\n")
		f("| Skill | Source | Path |\n|---|---|---|\n")
		for _, s := range unk {
			f("| `%s` | %s | `%s` |\n", s.Skill.Name, s.Skill.Source, s.Skill.Path)
		}
		f("\n")
	}

	// Ghost usage
	f("## Ghost usage\n\n")
	if len(r.GhostUsage) == 0 {
		f("_None — every invocation matched a discovered skill._\n\n")
	} else {
		f("Skills invoked in JSONL but not discovered. Likely missing `--root` coverage:\n\n")
		for _, g := range r.GhostUsage {
			f("- `%s`\n", g)
		}
		f("\n")
	}

	// Unmatched allowlist
	f("## Unmatched allowlist\n\n")
	if len(r.UnmatchedAllow) == 0 {
		f("_All allowlist entries matched a discovered skill._\n\n")
	} else {
		f("Allowlist entries with no matching skill. Likely renamed or stale:\n\n")
		for _, a := range r.UnmatchedAllow {
			f("- `%s`\n", a)
		}
		f("\n")
	}

	// Provenance
	f("## Provenance\n\n")
	f("- Roots: %s\n", strings.Join(r.Roots, ", "))
	f("- Globals included: %v\n", r.GlobalsIncluded)
	f("- Sessions parsed: %d\n", r.SessionsParsed)
	f("- omnisess version: %s\n", r.OmnisessVersion)
	return nil
}

func filterTier(rows []skills.SkillAudit, t skills.Tier) []skills.SkillAudit {
	var out []skills.SkillAudit
	for _, r := range rows {
		if r.Tier == t {
			out = append(out, r)
		}
	}
	return out
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.UTC().Format("2006-01-02")
}
