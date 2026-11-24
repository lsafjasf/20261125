package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
)

// sha256Hex returns the lowercase hex SHA-256 digest of everything read
// from r.
func sha256Hex(r io.Reader) (string, error) {
	digest := sha256.New()
	if _, err := io.Copy(digest, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// HashBytes returns the lowercase hex SHA-256 of b.
func HashBytes(b []byte) string {
	return sha256HexBytes(b)
}

func sha256HexBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
