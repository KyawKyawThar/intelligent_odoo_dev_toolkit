package utils

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashAPIKey returns the SHA-256 hex digest of an API key.
// SHA-256 is used instead of bcrypt because API key lookups require a
// deterministic hash that can be used as a database index.
func HashAPIKey(rawKey string) string {
	h := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(h[:])
}
