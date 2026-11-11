// contractvalidate validates a single bounded JSON document from standard input.
// It is a CI/differential-test entry point, not a user-facing diagnostic generator.
package main

import (
 "encoding/json"
 "flag"
 "fmt"
 "io"
 "os"

 "github.com/Herurg123/neurometr-agent/internal/contract"
)
func main() {
 root := flag.String("root", ".", "Agent checkout root containing immutable contracts")
 kind := flag.String("kind", "diagnostic", "diagnostic, catalog, manifest or pointer")
 flag.Parse()
 validator, err := contract.NewFullValidator(*root)
 if err != nil { fmt.Fprintln(os.Stderr, "validator setup:", err); os.Exit(2) }
 limit := contract.CatalogLimit
 if *kind == "diagnostic" { limit = contract.DiagnosticLimit }
 data, err := io.ReadAll(io.LimitReader(os.Stdin, int64(limit)+1))
 if err == nil {
  _, err = validator.ValidateBytes(*kind, data)
 }
 // Exit code always 0 for controlled validation success/rejection, 2 for setup.
 // CI cross-language driver compares this JSON outcome, not the error wording.
 outcome := struct {
  Valid bool `json:"valid"`
  Error string `json:"error,omitempty"`
 }{Valid: err == nil}
 if err != nil { outcome.Error = err.Error() }
 if json.NewEncoder(os.Stdout).Encode(outcome) != nil { os.Exit(2) }
}
