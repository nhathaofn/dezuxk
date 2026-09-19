package chrome

import "testing"

func liveInt(value int) *int { return &value }

func TestFlowQuotaFromProbeKeepsCombinedAndSeparateValuesDistinct(t *testing.T) {
	quota, err := flowQuotaFromProbe(flowQuotaProbeResult{
		Tier:           "pro",
		TotalCredits:   liveInt(1043),
		DailyCredits:   liveInt(43),
		MonthlyCredits: liveInt(1000),
		DailyResetAt:   "tomorrow",
	})
	if err != nil {
		t.Fatalf("flow quota parse failed: %v", err)
	}
	if !quota.Available || !quota.HasTotalCredits || quota.TotalCredits != 1043 {
		t.Fatalf("unexpected combined Flow quota: %+v", quota)
	}
	if !quota.HasDailyCredits || quota.DailyCredits != 43 {
		t.Fatalf("unexpected daily Flow quota: %+v", quota)
	}
	if !quota.HasMonthlyCredits || quota.MonthlyCredits != 1000 {
		t.Fatalf("unexpected monthly Flow quota: %+v", quota)
	}
}

func TestFlowQuotaFromProbeDoesNotInventMissingBuckets(t *testing.T) {
	quota, err := flowQuotaFromProbe(flowQuotaProbeResult{TotalCredits: liveInt(1043)})
	if err != nil {
		t.Fatalf("combined Flow quota should be valid: %v", err)
	}
	if !quota.HasTotalCredits || quota.HasDailyCredits || quota.HasMonthlyCredits {
		t.Fatalf("missing Flow buckets were fabricated: %+v", quota)
	}
	if quota.Message == "" {
		t.Fatal("expected an explicit unavailable-buckets message")
	}
}

func TestGeminiQuotaFromProbeCalculatesRemainingWindows(t *testing.T) {
	quota, err := geminiQuotaFromProbe(geminiQuotaProbeResult{
		Tier:               "PRO",
		CurrentUsedPercent: liveInt(1),
		WeeklyUsedPercent:  liveInt(0),
		CurrentResetAt:     "16:58",
		WeeklyResetAt:      "25 thg 9 lúc 11:58",
	})
	if err != nil {
		t.Fatalf("Gemini quota parse failed: %v", err)
	}
	if !quota.Available || quota.CurrentRemainingPercent != 99 || quota.WeeklyRemainingPercent != 100 {
		t.Fatalf("unexpected Gemini remaining windows: %+v", quota)
	}
	if !quota.HasCurrentRemaining || !quota.HasWeeklyRemaining {
		t.Fatalf("remaining flags were not set: %+v", quota)
	}
}

func TestGeminiQuotaFromProbeRejectsMissingData(t *testing.T) {
	quota, err := geminiQuotaFromProbe(geminiQuotaProbeResult{})
	if err == nil || quota.Available {
		t.Fatalf("missing Gemini quota should be unavailable: quota=%+v err=%v", quota, err)
	}
}

func TestLiveHostAllowed(t *testing.T) {
	if !liveHostAllowed("gemini.google.com", []string{"gemini.google.com"}) {
		t.Fatal("expected exact host to be allowed")
	}
	if !liveHostAllowed("sub.flow.google.com", []string{"flow.google.com"}) {
		t.Fatal("expected Google subdomain to be allowed")
	}
	if liveHostAllowed("evilgoogle.com", []string{"google.com"}) {
		t.Fatal("did not expect lookalike host to be allowed")
	}
}
