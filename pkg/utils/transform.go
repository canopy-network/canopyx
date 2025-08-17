package utils

import (
	"strings"

	"github.com/go-jose/go-jose/v4/json"
)

func GetString(m map[string]any, keys ...string) string {
	x := m
	for i, k := range keys {
		if i == len(keys)-1 {
			if v, ok := x[k].(string); ok {
				return v
			}
			return ""
		}
		n, _ := x[k].(map[string]any)
		x = n
	}
	return ""
}

func GetInt(m map[string]any, keys ...string) int64 {
	x := m
	for i, k := range keys {
		if i == len(keys)-1 {
			switch v := x[k].(type) {
			case float64:
				return int64(v)
			case int64:
				return v
			case json.Number:
				iv, _ := v.Int64()
				return iv
			}
			return 0
		}
		n, _ := x[k].(map[string]any)
		x = n
	}
	return 0
}

func BoolToUInt8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

func Dedup(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, e := range in {
		e = strings.TrimRight(e, "/")
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	return out
}

// ToUnixSeconds Normalize the incoming epoch to Unix seconds.
func ToUnixSeconds(ts int64) int64 {
	switch {
	case ts > 1e16: // nanoseconds
		return ts / 1e9
	case ts > 1e13: // microseconds
		return ts / 1e6
	case ts > 1e11: // milliseconds
		return ts / 1e3
	default: // seconds (or older timestamps)
		return ts
	}
}
