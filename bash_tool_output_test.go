package claude

import "testing"

func TestBashToolOutput_DecodesBackgroundFlags(t *testing.T) {
	var out BashToolOutput
	decodeToolUseResult(t, map[string]any{
		"stdout":                          "",
		"stderr":                          "",
		"interrupted":                     false,
		"backgroundTaskId":                "bash-1",
		"timedOutAfterMs":                 float64(120000),
		"backgroundEndsWithFinalResponse": true,
		"backgroundWakesOwner":            true,
	}, &out)

	if out.TimedOutAfterMs == nil || *out.TimedOutAfterMs != 120000 {
		t.Errorf("TimedOutAfterMs = %v, want 120000", out.TimedOutAfterMs)
	}
	if out.BackgroundEndsWithFinalResponse == nil || !*out.BackgroundEndsWithFinalResponse {
		t.Errorf("BackgroundEndsWithFinalResponse = %v, want true", out.BackgroundEndsWithFinalResponse)
	}
	if out.BackgroundWakesOwner == nil || !*out.BackgroundWakesOwner {
		t.Errorf("BackgroundWakesOwner = %v, want true", out.BackgroundWakesOwner)
	}
}

func TestBashToolOutput_BackgroundWakesOwnerAbsent(t *testing.T) {
	var out BashToolOutput
	decodeToolUseResult(t, map[string]any{"stdout": "ok", "interrupted": false}, &out)

	if out.BackgroundWakesOwner != nil {
		t.Errorf("BackgroundWakesOwner = %v, want nil when absent", *out.BackgroundWakesOwner)
	}
}
