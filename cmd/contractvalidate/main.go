// contractvalidate validates a single bounded JSON document from standard input.
// It is a CI/differential-test entry point, not a diagnostic generator.
package main

import (
 "encoding/json"
 "flag"
 "fmt"
 "io"
 "os"
 "path/filepath"

 "github.com/Herurg123/neurometr-agent/internal/contract"
)
func main() {
 root := flag.String("root", ".", "Agent checkout root containing immutable contracts")
 kind := flag.String("kind", "diagnostic", "diagnostic, catalog, manifest, pointer, numeric or snapshot")
 signatureVerified := flag.Bool("signature-verified", false, "test-only simulated trust result for the F07 snapshot")
 flag.Parse()
 validator, err := contract.NewFullValidator(*root)
 if err != nil { fmt.Fprintln(os.Stderr, "validator setup:", err); os.Exit(2) }
 limit := contract.CatalogLimit
 if *kind == "diagnostic" { limit = contract.DiagnosticLimit }
 data, err := io.ReadAll(io.LimitReader(os.Stdin, int64(limit)+1))
 if err == nil {
  switch *kind {
  case "numeric":
   _, err = contract.DecodeBounded(data, contract.DiagnosticLimit)
  case "snapshot":
   // Exactly F07's pinned synthetic manifest; NOT a general signature verifier.
   manifestBytes, readErr := os.ReadFile(filepath.Join(*root, "contracts", "fixtures", "F07_catalog_invalid_checksum.manifest.json"))
   if readErr != nil { fmt.Fprintln(os.Stderr, readErr); os.Exit(2) }
   var manifest map[string]any
   manifest, err = validator.ValidateBytes("manifest", manifestBytes)
   if err == nil { _, err = validator.ValidateBytes("catalog", data) }
   if err == nil { err = contract.SnapshotIntegrity(manifest, data, *signatureVerified) }
  default:
   _, err = validator.ValidateBytes(*kind, data)
  }
 }
 // Exit 0 for expected validation outcomes; 2 for setup failures.
 // The cross-language driver compares this JSON outcome, not error wording.
 outcome := struct {
  Valid bool `json:"valid"`
  Error string `json:"error,omitempty"`
 }{Valid: err == nil}
 if err != nil { outcome.Error = err.Error() }
 if json.NewEncoder(os.Stdout).Encode(outcome) != nil { os.Exit(2) }
}
