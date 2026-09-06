package conformance

import (
	"crypto/sha1"
	"fmt"
	"os"
	"regexp"
	"testing"
)

var securityEvidencePinPattern = regexp.MustCompile(`(?m)^security-evidence-blob: ([0-9a-f]{40})$`)

func gitBlobSHA(content []byte) string {
	h := sha1.New()
	_, _ = fmt.Fprintf(h, "blob %d\x00", len(content))
	_, _ = h.Write(content)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func TestOmarchySubmissionPinsCurrentSecurityEvidenceBlob(t *testing.T) {
	proposal, err := os.ReadFile("../docs/omarchy-submission.md")
	if err != nil {
		t.Fatal(err)
	}
	matches := securityEvidencePinPattern.FindAllSubmatch(proposal, -1)
	if len(matches) != 1 {
		t.Fatalf("docs/omarchy-submission.md must contain exactly one machine-readable security-evidence-blob pin; found %d", len(matches))
	}

	ledger, err := os.ReadFile("../docs/security-evidence.md")
	if err != nil {
		t.Fatal(err)
	}
	want := gitBlobSHA(ledger)
	got := string(matches[0][1])
	if got != want {
		t.Fatalf("stale security evidence pin: proposal=%s current_blob=%s; update the proposal pin in the same change as docs/security-evidence.md", got, want)
	}
}
