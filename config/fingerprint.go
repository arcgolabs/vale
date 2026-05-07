package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/samber/oops"
)

// Fingerprint returns a stable content hash for a fully materialized config.
func Fingerprint(cfg *Config) (string, error) {
	if cfg == nil {
		return "", nil
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", oops.In("config").Wrapf(err, "fingerprint config")
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
