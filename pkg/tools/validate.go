// Package tools — JSON Schema validation for tool call arguments.
//
// ValidateAndCoerce validates the arguments provided by the LLM against the
// tool's declared JSON Schema, coercing simple type mismatches (e.g. "5" → 5)
// and returning a clear error message when validation fails.
package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// ValidateAndCoerce validates args against the JSON Schema stored in tool's
// Parameters field. It returns the (possibly coerced) arguments or a
// descriptive error.
//
// Coercion rules (matching what LLMs commonly get wrong):
//   - A JSON string containing a valid number is coerced to float64/int64 when
//     the schema expects "number" or "integer".
//   - A JSON number is coerced to string when the schema expects "string".
//   - A string "true"/"false" is coerced to bool when the schema expects "boolean".
//
// If the schema cannot be compiled, args are returned unchanged (fail open).
func ValidateAndCoerce(t Tool, args map[string]any) (map[string]any, error) {
	schemaBytes := t.Definition().Parameters
	if len(schemaBytes) == 0 {
		return args, nil
	}

	schema, err := compileSchema(schemaBytes)
	if err != nil {
		// Unparseable schema — fail open so callers don't break on bad schemas.
		return args, nil
	}

	// First attempt: validate as-is.
	if err := validateMap(schema, args); err == nil {
		return args, nil
	}

	// Second attempt: coerce obvious type mismatches and retry.
	coerced := coerceArgs(args, schemaBytes)
	if err := validateMap(schema, coerced); err == nil {
		return coerced, nil
	} else {
		return nil, formatValidationError(t.Definition().Name, args, err)
	}
}

// compileSchema unmarshals the schema bytes and compiles them.
// A fresh compiler is used each time to avoid resource-collision errors.
func compileSchema(schemaBytes []byte) (*jsonschema.Schema, error) {
	// jsonschema/v6 requires an already-unmarshaled value for AddResource.
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaBytes))
	if err != nil {
		return nil, fmt.Errorf("unmarshal schema: %w", err)
	}

	c := jsonschema.NewCompiler()
	const url = "mem://tool/schema"
	if err := c.AddResource(url, doc); err != nil {
		return nil, fmt.Errorf("add schema resource: %w", err)
	}
	return c.Compile(url)
}

// validateMap marshals the map to JSON and validates it against the schema.
func validateMap(schema *jsonschema.Schema, args map[string]any) error {
	b, err := json.Marshal(args)
	if err != nil {
		return err
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		return err
	}
	return schema.Validate(inst)
}

// coerceArgs attempts simple type coercions on top-level properties.
func coerceArgs(args map[string]any, schemaBytes []byte) map[string]any {
	var schemaDef struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	_ = json.Unmarshal(schemaBytes, &schemaDef)

	out := make(map[string]any, len(args))
	for k, v := range args {
		prop, ok := schemaDef.Properties[k]
		if !ok {
			out[k] = v
			continue
		}
		out[k] = coerceValue(v, prop.Type)
	}
	return out
}

func coerceValue(v any, targetType string) any {
	switch targetType {
	case "number", "integer":
		// String → number (LLMs sometimes quote numeric values)
		if s, ok := v.(string); ok {
			var n float64
			if err := json.Unmarshal([]byte(s), &n); err == nil {
				if targetType == "integer" {
					return int64(n)
				}
				return n
			}
		}
	case "string":
		// Number → string
		switch n := v.(type) {
		case float64:
			return fmt.Sprintf("%g", n)
		case int64:
			return fmt.Sprintf("%d", n)
		case json.Number:
			return n.String()
		}
	case "boolean":
		// String → bool
		if s, ok := v.(string); ok {
			switch strings.ToLower(s) {
			case "true":
				return true
			case "false":
				return false
			}
		}
	}
	return v
}

func formatValidationError(toolName string, args map[string]any, err error) error {
	argsJSON, _ := json.MarshalIndent(args, "", "  ")
	return fmt.Errorf("tool %q argument validation failed:\n%v\n\nReceived:\n%s",
		toolName, err, argsJSON)
}
