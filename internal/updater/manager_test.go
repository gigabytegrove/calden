package updater

import (
	"testing"
	"time"
)

func TestCompareVersionsOrdersAlphaNumericSuffixNaturally(t *testing.T) {
	tests := []struct {
		left, right string
		want        int
	}{
		{"0.1.0-alpha10", "0.1.0-alpha9", 1},
		{"0.1.0-alpha11", "0.1.0-alpha10", 1},
		{"0.1.0-alpha9.1", "0.1.0-alpha9", 1},
		{"0.1.0-alpha10.1", "0.1.0-alpha9.1", 1},
		{"0.1.0-beta2", "0.1.0-beta10", -1},
		{"0.1.0-alpha10", "0.1.0-alpha10", 0},
	}
	for _, test := range tests {
		got := compareVersions(test.left, test.right)
		if got < 0 {
			got = -1
		} else if got > 0 {
			got = 1
		}
		if got != test.want {
			t.Fatalf("compareVersions(%q,%q)=%d, want %d", test.left, test.right, got, test.want)
		}
	}
}

func TestLatestEligibleSelectsAlpha10OverAlpha9(t *testing.T) {
	releases := []releasePayload{
		{TagName: "v0.1.0-alpha9", Prerelease: true, PublishedAt: time.Now().Add(-time.Hour)},
		{TagName: "v0.1.0-alpha10", Prerelease: true, PublishedAt: time.Now()},
	}
	latest := latestEligible(releases, "alpha")
	if latest == nil {
		t.Fatal("expected eligible alpha release")
	}
	if latest.TagName != "v0.1.0-alpha10" {
		t.Fatalf("latestEligible chose %s, want v0.1.0-alpha10", latest.TagName)
	}
}

func TestBridgeReleaseIsVisibleToBrokenAlpha9Sequence(t *testing.T) {
	// The bridge release intentionally uses alpha9.1 so older alpha9 builds,
	// which compare prerelease identifiers lexically, can still discover it.
	if compareVersions("0.1.0-alpha9.1", "0.1.0-alpha9") <= 0 {
		t.Fatal("alpha9.1 bridge must sort above alpha9")
	}
}
