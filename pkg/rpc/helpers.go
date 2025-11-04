package rpc

// Helper functions to safely extract fields from map[string]interface{}

// GetStringField retrieves the string value for the given key from a map. Returns an empty string if the key is absent or not a string.
func GetStringField(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetIntField retrieves an integer value from a map for the specified key, supporting int, int64, and float64 conversions.
// If the key is not found or the value is not a supported type, it returns 0.
func GetIntField(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case int64:
			return int(val)
		case float64:
			return int(val)
		}
	}
	return 0
}

// GetOptionalUint32Field retrieves an optional uint32 value from a map by its key, returning nil if the key is missing or invalid.
func GetOptionalUint32Field(m map[string]interface{}, key string) *uint32 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			u := uint32(val)
			return &u
		case int64:
			u := uint32(val)
			return &u
		case float64:
			u := uint32(val)
			return &u
		case uint32:
			return &val
		}
	}
	return nil
}

// GetOptionalUint64Field retrieves an optional uint64 value from a map if the key exists and the value is a numeric type.
func GetOptionalUint64Field(m map[string]interface{}, key string) *uint64 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			u := uint64(val)
			return &u
		case int64:
			u := uint64(val)
			return &u
		case float64:
			u := uint64(val)
			return &u
		case uint64:
			return &val
		}
	}
	return nil
}

// GetFloat64Field retrieves a float64 value from a map using the specified key, converting types if necessary.
// Supported conversions: float32, int, int64 to float64.
// Returns 0.0 if the key is not found or the value cannot be converted.
func GetFloat64Field(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return val
		case float32:
			return float64(val)
		case int:
			return float64(val)
		case int64:
			return float64(val)
		}
	}
	return 0.0
}

// GetOptionalStringField retrieves a string value from a map by key, returning a pointer to the string or nil if not found.
func GetOptionalStringField(m map[string]interface{}, key string) *string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return &s
		}
	}
	return nil
}

// GetOptionalBoolField retrieves a boolean value from the provided map using the given key and returns a pointer to it.
// Returns nil if the key does not exist or the value is not a boolean.
func GetOptionalBoolField(m map[string]interface{}, key string) *bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return &b
		}
	}
	return nil
}
