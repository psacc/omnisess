// Package audit joins skill inventory + usage + allowlist into a tiered audit result.
package audit

import (
	"sort"
	"strings"
	"time"

	"github.com/psacc/omnisess/internal/skills"
)

// TierInput is the input to Tier.
type TierInput struct {
	Skills      []skills.SkillInfo
	Invocations []skills.Invocation
	Allowlist   []string
	Window      time.Duration
	Now         time.Time
}

// Tier classifies each skill and records ghost usage / unmatched allowlist entries.
func Tier(in TierInput) skills.AuditResult {
	cutoff := in.Now.Add(-in.Window)

	// Index skills by name.
	byName := make(map[string]skills.SkillInfo, len(in.Skills))
	for _, s := range in.Skills {
		byName[s.Name] = s
	}

	allowSet := make(map[string]struct{}, len(in.Allowlist))
	for _, a := range in.Allowlist {
		allowSet[a] = struct{}{}
	}

	type counts struct {
		model, user int
		last        time.Time
	}
	invCounts := make(map[string]*counts)
	var ghosts []string
	seenGhost := map[string]struct{}{}

	for _, inv := range in.Invocations {
		if inv.Timestamp.Before(cutoff) {
			continue
		}
		matched := inv.SkillName
		if _, known := byName[matched]; !known {
			// Fallback: strip "namespace:" prefix and match the bare suffix.
			if i := strings.Index(matched, ":"); i >= 0 {
				bare := matched[i+1:]
				if _, known := byName[bare]; known {
					matched = bare
				}
			}
		}
		if _, known := byName[matched]; !known {
			if _, dup := seenGhost[inv.SkillName]; !dup {
				ghosts = append(ghosts, inv.SkillName)
				seenGhost[inv.SkillName] = struct{}{}
			}
			continue
		}
		c, ok := invCounts[matched]
		if !ok {
			c = &counts{}
			invCounts[matched] = c
		}
		switch inv.Kind {
		case skills.InvocationModel:
			c.model++
		case skills.InvocationUser:
			c.user++
		}
		if inv.Timestamp.After(c.last) {
			c.last = inv.Timestamp
		}
	}

	var auditRows []skills.SkillAudit
	for _, s := range in.Skills {
		c := invCounts[s.Name]
		var row skills.SkillAudit
		row.Skill = s
		if c != nil {
			row.ModelInvoked = c.model
			row.UserInvoked = c.user
			row.LastUsed = c.last
		}
		row.Total = row.ModelInvoked + row.UserInvoked
		_, allowed := allowSet[s.Name]
		row.Allowlisted = allowed

		switch {
		case isUnknownSource(s.Source):
			row.Tier = skills.TierUnknown
		case allowed:
			row.Tier = skills.TierKeep
		case row.Total >= 3:
			row.Tier = skills.TierKeep
		case row.Total >= 1:
			row.Tier = skills.TierBorderline
		default:
			row.Tier = skills.TierArchive
		}
		auditRows = append(auditRows, row)
	}

	sort.SliceStable(auditRows, func(i, j int) bool {
		ti, tj := tierOrder(auditRows[i].Tier), tierOrder(auditRows[j].Tier)
		if ti != tj {
			return ti < tj
		}
		return auditRows[i].Total > auditRows[j].Total
	})

	var unmatched []string
	for _, a := range in.Allowlist {
		if _, ok := byName[a]; !ok {
			unmatched = append(unmatched, a)
		}
	}

	return skills.AuditResult{
		Window:         in.Window,
		GeneratedAt:    in.Now,
		Skills:         auditRows,
		GhostUsage:     ghosts,
		UnmatchedAllow: unmatched,
	}
}

func isUnknownSource(s skills.Source) bool {
	return s == skills.SourceCodexProject || s == skills.SourceCodexGlobal
}

func tierOrder(t skills.Tier) int {
	switch t {
	case skills.TierKeep:
		return 0
	case skills.TierBorderline:
		return 1
	case skills.TierArchive:
		return 2
	case skills.TierUnknown:
		return 3
	}
	return 4
}
