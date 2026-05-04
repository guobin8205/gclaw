package tool

// NormalizeParams wraps bare scalars in single-element lists for array-typed parameters.
// Some LLMs send single values instead of arrays for parameters defined as type "array".
func NormalizeParams(schema Schema, params map[string]any) {
	for key, prop := range schema.Properties {
		if prop.Type != "array" {
			continue
		}
		val, exists := params[key]
		if !exists {
			continue
		}
		// If val is already a slice, leave it alone
		if _, ok := val.([]any); ok {
			continue
		}
		// If val is nil, leave it alone
		if val == nil {
			continue
		}
		// Wrap the bare scalar in a single-element list
		params[key] = []any{val}
	}
}
