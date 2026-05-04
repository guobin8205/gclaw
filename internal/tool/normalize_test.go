package tool

import "testing"

func TestNormalizeParams(t *testing.T) {
	t.Run("bare string for array param gets wrapped", func(t *testing.T) {
		schema := Schema{
			Properties: map[string]Property{
				"items": {Type: "array", Items: &Property{Type: "string"}},
			},
		}
		params := map[string]any{"items": "single_value"}
		NormalizeParams(schema, params)

		got, ok := params["items"].([]any)
		if !ok {
			t.Fatalf("expected []any, got %T", params["items"])
		}
		if len(got) != 1 || got[0] != "single_value" {
			t.Fatalf("expected [single_value], got %v", got)
		}
	})

	t.Run("already a []any is unchanged", func(t *testing.T) {
		schema := Schema{
			Properties: map[string]Property{
				"items": {Type: "array", Items: &Property{Type: "string"}},
			},
		}
		original := []any{"a", "b"}
		params := map[string]any{"items": original}
		NormalizeParams(schema, params)

		got, ok := params["items"].([]any)
		if !ok {
			t.Fatalf("expected []any, got %T", params["items"])
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 elements, got %d", len(got))
		}
	})

	t.Run("non-array param is unchanged", func(t *testing.T) {
		schema := Schema{
			Properties: map[string]Property{
				"name": {Type: "string"},
			},
		}
		params := map[string]any{"name": "hello"}
		NormalizeParams(schema, params)

		if params["name"] != "hello" {
			t.Fatalf("expected 'hello', got %v", params["name"])
		}
	})

	t.Run("nil value for array param is unchanged", func(t *testing.T) {
		schema := Schema{
			Properties: map[string]Property{
				"items": {Type: "array", Items: &Property{Type: "string"}},
			},
		}
		params := map[string]any{"items": nil}
		NormalizeParams(schema, params)

		if params["items"] != nil {
			t.Fatalf("expected nil, got %v", params["items"])
		}
	})

	t.Run("missing param is unchanged", func(t *testing.T) {
		schema := Schema{
			Properties: map[string]Property{
				"items": {Type: "array", Items: &Property{Type: "string"}},
			},
		}
		params := map[string]any{"other": 42}
		NormalizeParams(schema, params)

		if _, exists := params["items"]; exists {
			t.Fatal("expected 'items' to not exist in params")
		}
	})

	t.Run("bare int for array param gets wrapped", func(t *testing.T) {
		schema := Schema{
			Properties: map[string]Property{
				"ids": {Type: "array", Items: &Property{Type: "integer"}},
			},
		}
		params := map[string]any{"ids": 42}
		NormalizeParams(schema, params)

		got, ok := params["ids"].([]any)
		if !ok {
			t.Fatalf("expected []any, got %T", params["ids"])
		}
		if len(got) != 1 || got[0] != 42 {
			t.Fatalf("expected [42], got %v", got)
		}
	})

	t.Run("mixed array and non-array params", func(t *testing.T) {
		schema := Schema{
			Properties: map[string]Property{
				"files":   {Type: "array", Items: &Property{Type: "string"}},
				"options": {Type: "array", Items: &Property{Type: "string"}},
				"name":    {Type: "string"},
			},
		}
		params := map[string]any{
			"files":   "readme.md",
			"options": []any{"verbose"},
			"name":    "test",
		}
		NormalizeParams(schema, params)

		// files should be wrapped
		files, ok := params["files"].([]any)
		if !ok {
			t.Fatalf("expected files to be []any, got %T", params["files"])
		}
		if len(files) != 1 || files[0] != "readme.md" {
			t.Fatalf("expected files [readme.md], got %v", files)
		}

		// options should stay as-is
		opts, ok := params["options"].([]any)
		if !ok {
			t.Fatalf("expected options to be []any, got %T", params["options"])
		}
		if len(opts) != 1 || opts[0] != "verbose" {
			t.Fatalf("expected options [verbose], got %v", opts)
		}

		// name should stay as-is
		if params["name"] != "test" {
			t.Fatalf("expected name 'test', got %v", params["name"])
		}
	})
}
