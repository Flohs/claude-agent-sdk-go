package claude

import (
	"strings"
	"testing"
)

func TestUsageLimitPrefixes_MatchExpectedMessages(t *testing.T) {
	tests := []struct {
		name     string
		prefixes []string
		message  string
	}{
		{"org policy limit", OrgPolicyLimitPrefixes, "This service is disabled for your org policy settings"},
		{"usage limit error", UsageLimitErrorPrefixes, "You've hit your usage limit for this session"},
		{"usage transition", UsageTransitionPrefixes, "You're now using usage credits for this session"},
		{"usage warning", UsageWarningPrefixes, "You've used 90% of your allocation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := false
			for _, prefix := range tt.prefixes {
				if strings.HasPrefix(tt.message, prefix) {
					matched = true
					break
				}
			}
			if !matched {
				t.Errorf("expected %q to match one of %v", tt.message, tt.prefixes)
			}
		})
	}
}

func TestUsageLimitPrefixes_NonEmpty(t *testing.T) {
	buckets := map[string][]string{
		"OrgPolicyLimitPrefixes":  OrgPolicyLimitPrefixes,
		"UsageLimitErrorPrefixes": UsageLimitErrorPrefixes,
		"UsageTransitionPrefixes": UsageTransitionPrefixes,
		"UsageWarningPrefixes":    UsageWarningPrefixes,
	}
	for name, bucket := range buckets {
		if len(bucket) == 0 {
			t.Errorf("%s should not be empty", name)
		}
	}
}
