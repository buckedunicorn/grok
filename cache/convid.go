package cache

import (
	"crypto/rand"
	"encoding/hex"
)

// newConvID generates a random 16-byte hex conversation ID.
func newConvID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("cache: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}
