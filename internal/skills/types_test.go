package skills

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTierStringRoundTrip(t *testing.T) {
	cases := []Tier{TierKeep, TierBorderline, TierArchive, TierUnknown}
	for _, c := range cases {
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatalf("marshal %v: %v", c, err)
		}
		var got Tier
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshal %v: %v", c, err)
		}
		if got != c {
			t.Errorf("round trip: got %v want %v", got, c)
		}
	}
}

func TestSkillAuditTotalIsSum(t *testing.T) {
	sa := SkillAudit{ModelInvoked: 3, UserInvoked: 5}
	sa.Total = sa.ModelInvoked + sa.UserInvoked
	if sa.Total != 8 {
		t.Errorf("Total: got %d want 8", sa.Total)
	}
}

func TestInvocationKindValues(t *testing.T) {
	if InvocationModel == InvocationUser {
		t.Fatal("kinds must be distinct")
	}
	if string(InvocationModel) != "model" || string(InvocationUser) != "user" {
		t.Errorf("InvocationKind values changed; check JSON contracts")
	}
}

func TestAuditResultZeroValueIsValid(t *testing.T) {
	var r AuditResult
	if r.GeneratedAt != (time.Time{}) {
		t.Errorf("zero AuditResult should have zero time")
	}
	if len(r.Skills) != 0 {
		t.Errorf("zero AuditResult should have empty Skills")
	}
}
