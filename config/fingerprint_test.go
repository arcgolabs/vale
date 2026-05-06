package config

import "testing"

func TestFingerprintStableForSameConfig(t *testing.T) {
	t.Parallel()

	left := Default()
	right := Default()
	leftHash, err := Fingerprint(left)
	if err != nil {
		t.Fatal(err)
	}
	rightHash, err := Fingerprint(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftHash == "" || leftHash != rightHash {
		t.Fatalf("fingerprints = %q and %q, want same non-empty hash", leftHash, rightHash)
	}

	right.Admin.Address = ":19091"
	changedHash, err := Fingerprint(right)
	if err != nil {
		t.Fatal(err)
	}
	if changedHash == leftHash {
		t.Fatalf("changed fingerprint = %q, want different from %q", changedHash, leftHash)
	}
}
