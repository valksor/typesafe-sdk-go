package typesafe

import "math"

// nonFiniteString returns the string JSON stand-in for a NaN or infinite
// number; encoding/json cannot encode them.
func nonFiniteString(f float64) (string, bool) {
	switch {
	case math.IsNaN(f):
		return "NaN", true
	case math.IsInf(f, 1):
		return "Infinity", true
	case math.IsInf(f, -1):
		return "-Infinity", true
	}
	return "", false
}

// sanitizeNonFinite replaces NaN, +Inf, and -Inf with the strings "NaN",
// "Infinity", and "-Infinity" anywhere inside float32/float64 values and
// []float64, []float32, []any, and map[string]any containers. Other values,
// including finite numbers and caller-defined types, are returned unchanged,
// so a caller-defined struct holding a non-finite float still fails to encode.
func sanitizeNonFinite(value JSONValue) JSONValue {
	switch v := value.(type) {
	case float64:
		if s, ok := nonFiniteString(v); ok {
			return s
		}
	case float32:
		if s, ok := nonFiniteString(float64(v)); ok {
			return s
		}
	case []float64:
		return sanitizeFloats(v)
	case []float32:
		return sanitizeFloats(v)
	case []JSONValue:
		if v == nil {
			return v
		}
		out := make([]JSONValue, len(v))
		for i, item := range v {
			out[i] = sanitizeNonFinite(item)
		}
		return out
	case map[string]JSONValue:
		if v == nil {
			return v
		}
		out := make(map[string]JSONValue, len(v))
		for key, item := range v {
			out[key] = sanitizeNonFinite(item)
		}
		return out
	}
	return value
}

// sanitizeFloats returns the slice unchanged unless it holds a non-finite
// value, in which case it returns a []any with those values stringified.
func sanitizeFloats[F float32 | float64](values []F) JSONValue {
	for _, f := range values {
		if _, bad := nonFiniteString(float64(f)); bad {
			out := make([]JSONValue, len(values))
			for i, item := range values {
				out[i] = sanitizeNonFinite(item)
			}
			return out
		}
	}
	return values
}

// sanitized returns a copy of the criteria with non-finite numbers stringified.
func (c *NoulCriteria) sanitized() *NoulCriteria {
	if c == nil {
		return nil
	}
	return &NoulCriteria{True: sanitizeNonFinite(c.True), False: sanitizeNonFinite(c.False)}
}
