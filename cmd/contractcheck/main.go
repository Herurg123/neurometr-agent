// contractcheck verifies the A1 immutable Web contract pin in a GitHub runner.
package main

import (
 "flag"
 "fmt"
 "os"
 "github.com/Herurg123/neurometr-agent/internal/contract"
)

func main() {
 root:=flag.String("root",".","repository root")
 flag.Parse()
 if err:=contract.VerifyPin(*root);err!=nil{fmt.Fprintln(os.Stderr,"PIN FAIL:",err);os.Exit(1)}
 if err:=contract.VerifySchemaHeaders(*root);err!=nil{fmt.Fprintln(os.Stderr,"SCHEMA HEADER FAIL:",err);os.Exit(1)}
 paths,err:=contract.ExpectedFiles(*root);if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
 fmt.Printf("A1 PIN VERIFIED: %d canonical files; Web commit %s; contract %s; SHA-256 exact bytes\n",len(paths),contract.SourceCommit,contract.Version)
}
