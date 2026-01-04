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
