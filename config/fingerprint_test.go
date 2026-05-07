package config_test

import (
	"testing"

	"github.com/arcgolabs/vela/config"
)

func TestFingerprintStableForSameConfig(t *testing.T) {
	t.Parallel()

	left := config.Default()
	right := config.Default()
	leftHash, err := config.Fingerprint(left)
	if err != nil {
		t.Fatal(err)
	}
	rightHash, err := config.Fingerprint(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftHash == "" || leftHash != rightHash {
		t.Fatalf("fingerprints = %q and %q, want same non-empty hash", leftHash, rightHash)
	}

	right.Admin.Address = ":19091"
	changedHash, err := config.Fingerprint(right)
	if err != nil {
		t.Fatal(err)
	}
	if changedHash == leftHash {
		t.Fatalf("changed fingerprint = %q, want different from %q", changedHash, leftHash)
	}
}
