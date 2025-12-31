package transform

import "encoding/hex"

// bytesToHex converts a byte slice to a hex string.
// Returns empty string if byte slice is empty.
func bytesToHex(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return hex.EncodeToString(b)
}

// ptrHex converts a byte slice to a pointer to hex string.
// Returns nil if byte slice is empty, otherwise returns pointer to hex string.
// This is useful for optional fields in database models.
func ptrHex(b []byte) *string {
	if len(b) == 0 {
		return nil
	}
	s := hex.EncodeToString(b)
	return &s
}
