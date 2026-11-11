package contract

import (
 "bytes"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"

 jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// FullValidator validates the exact pinned B1 canonical Draft 2020-12
// schemas followed by the separately approved Go semantic checks.
// All schema resources are local; imported JSON never supplies a schema URI.
type FullValidator struct {
 schemas map[string]*jsonschema.Schema
}

var fullSchemaNames = map[string]string{
 "diagnostic": "diagnostic.schema.json",
 "catalog": "catalog.schema.json",
 "manifest": "catalog-manifest.schema.json",
 "pointer": "latest-pointer.schema.json",
}

// NewFullValidator compiles canonical schemas, with format assertions enabled.
// Pin verification is required before any local schema is trusted.
// Compilation never uses a remote schema resource.
func NewFullValidator(root string) (*FullValidator, error) {
 if err := VerifyPin(root); err != nil { return nil, fmt.Errorf("contract pin: %w", err) }
 if err := VerifySchemaHeaders(root); err != nil { return nil, fmt.Errorf("schema headers: %w", err) }
 compiler := jsonschema.NewCompiler()
 compiler.AssertFormat()
 ids := make(map[string]string, len(fullSchemaNames))
 for kind, file := range fullSchemaNames {
  raw, err := os.ReadFile(filepath.Join(root, "contracts", file))
  if err != nil { return nil, err }
  // The approved v0.1.0 schemas contain only same-document fragment refs.
  // Never fetch an external URI, even if a future schema unexpectedly adds one.
  resource, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
  if err != nil { return nil, fmt.Errorf("%s: invalid schema JSON: %w", file, err) }
  m, ok := resource.(map[string]any)
  if !ok { return nil, fmt.Errorf("%s: schema root must be an object", file) }
  id, ok := m["$id"].(string)
  if !ok || !strings.HasPrefix(id, "urn:neurometr:schema:") {
   return nil, fmt.Errorf("%s: unexpected schema $id", file)
  }
  // Freeze references: v0.1.0 canonical documents must not import code or remote data.
  if err := localRefsOnly(resource); err != nil { return nil, fmt.Errorf("%s: %w", file, err) }
  if err := compiler.AddResource(id, resource); err != nil { return nil, err }
  ids[kind] = id
 }
 schemas := make(map[string]*jsonschema.Schema, len(fullSchemaNames))
 for kind, id := range ids {
  schema, err := compiler.Compile(id)
  if err != nil { return nil, fmt.Errorf("compile %s: %w", kind, err) }
  schemas[kind] = schema
 }
 return &FullValidator{schemas: schemas}, nil
}

func localRefsOnly(v any) error {
 switch v := v.(type) {
 case map[string]any:
  for key, child := range v {
   if key == "$ref" || key == "$dynamicRef" {
    s, ok := child.(string)
    if !ok || !strings.HasPrefix(s, "#") {
     return errors.New("non-local schema reference is forbidden")
    }
   }
   if err := localRefsOnly(child); err != nil { return err }
  }
 case []any:
  for _, child := range v {
   if err := localRefsOnly(child); err != nil { return err }
  }
 }
 return nil
}

// ValidateBytes applies: strict UTF-8, duplicate-key, numeric, size,
// depth/node budgets -> canonical schema (including formats) -> semantics.
// It returns a decoded object only when every layer accepts the input.
func (v *FullValidator) ValidateBytes(kind string, raw []byte) (map[string]any, error) {
 schema, ok := v.schemas[kind]
 if !ok { return nil, fmt.Errorf("unsupported contract kind %q", kind) }
 limit := CatalogLimit
 if kind == "diagnostic" { limit = DiagnosticLimit }
 parsed, err := DecodeBounded(raw, limit)
 if err != nil { return nil, fmt.Errorf("strict JSON: %w", err) }
 obj, ok := parsed.(map[string]any)
 if !ok { return nil, errors.New("contract root must be an object") }
 if obj["schema_version"] != Version { return nil, errors.New("unsupported schema version") }
 if err := schema.Validate(parsed); err != nil { return nil, fmt.Errorf("%s Draft 2020-12: %w", kind, err) }
 // Reuse existing selected semantic invariants instead of reimplementing
 // them in a JSON Schema engine. The full schema already checked field shapes.
 if _, err := Validate(kind, raw); err != nil { return nil, fmt.Errorf("%s semantics: %w", kind, err) }
 return obj, nil
}
