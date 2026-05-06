package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Fingerprint returns a stable content hash for a fully materialized config.
func Fingerprint(cfg *Config) (string, error) {
	if cfg == nil {
		return "", nil
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
