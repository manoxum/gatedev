package shaping

import "testing"

func TestFindRuleLineByCommentTargetMatchesOnlyLegacyReturnRule(t *testing.T) {
	listing := `
Chain BINDNET-SHAPING (1 references)
num  target     prot opt source               destination
1    RETURN     all  --  0.0.0.0/0            0.0.0.0/0            /* bn-global-up */
2               all  --  0.0.0.0/0            0.0.0.0/0            /* bn-global-down */
3    MARK       all  --  0.0.0.0/0            0.0.0.0/0            /* bn-up-aa:bb:cc:dd:ee:ff */ MARK set 0x2
`

	if got := findRuleLineByCommentTarget(listing, "bn-global-up", "RETURN"); got != "1" {
		t.Fatalf("legacy RETURN line = %q, want 1", got)
	}
	if got := findRuleLineByCommentTarget(listing, "bn-global-down", "RETURN"); got != "" {
		t.Fatalf("new counter-only global rule matched RETURN line %q, want empty", got)
	}
	if got := findRuleLineByCommentTarget(listing, "bn-up-aa:bb:cc:dd:ee:ff", "RETURN"); got != "" {
		t.Fatalf("device MARK rule matched RETURN line %q, want empty", got)
	}
}
