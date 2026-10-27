package updater

import (
	"encoding/json"
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
		testReadyRelease(t, "v0.1.0-alpha9", time.Now().Add(-time.Hour)),
		testReadyRelease(t, "v0.1.0-alpha10", time.Now()),
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

func testReadyRelease(t *testing.T, tag string, published time.Time) releasePayload {
	t.Helper()
	var release releasePayload
	payload := `{"assets":[{"name":"calden-linux-amd64.tar.gz","browser_download_url":"https://example.test/amd64"},{"name":"calden-linux-arm64.tar.gz","browser_download_url":"https://example.test/arm64"},{"name":"SHA256SUMS","browser_download_url":"https://example.test/sums"}]}`
	if err := json.Unmarshal([]byte(payload), &release); err != nil {
		t.Fatal(err)
	}
	release.TagName = tag
	release.Prerelease = true
	release.PublishedAt = published
	return release
}
func TestLatestEligibleSkipsBrokenPublishedReleases(t *testing.T) {
	complete := testReadyRelease(t, "v1.5.0-alpha.7", time.Now().Add(-2*time.Hour))
	missingAssets := releasePayload{TagName: "v1.5.0-alpha.9", Prerelease: true, PublishedAt: time.Now()}
	missingChecksums := testReadyRelease(t, "v1.5.0-alpha.8", time.Now().Add(-time.Hour))
	missingChecksums.Assets = missingChecksums.Assets[:2]
	got := latestEligible([]releasePayload{complete, missingAssets, missingChecksums}, "alpha")
	if got == nil || got.TagName != complete.TagName {
		t.Fatalf("assetless release was offered as installable: %#v", got)
	}
	if releaseReadyForInstall(missingAssets) || releaseReadyForInstall(missingChecksums) {
		t.Fatal("broken release has been incorrectly accepted")
	}
	if !releaseReadyForInstall(complete) {
		t.Fatal("valid release incorrectly rejected")
	}
}
