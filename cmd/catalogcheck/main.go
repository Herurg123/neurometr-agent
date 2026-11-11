// catalogcheck is a read-only A3.3 build/CI gate for the approved offline pin.
package main

import (
 "flag"
 "fmt"
 "os"

 "github.com/Herurg123/neurometr-agent/internal/catalogpin"
 "github.com/Herurg123/neurometr-agent/internal/contract"
)
func main(){
 root:=flag.String("root",".","source root containing pinned contracts and bundled catalog")
 flag.Parse()
 v,err:=contract.NewFullValidator(*root)
 if err!=nil{fmt.Fprintln(os.Stderr,"A3_3_CATALOG_PIN=FAIL: canonical contract pin invalid");os.Exit(1)}
 ref,err:=catalogpin.Verify(*root,v)
 if err!=nil{fmt.Fprintln(os.Stderr,"A3_3_CATALOG_PIN=FAIL: approved bundled catalog identity or schema mismatch");os.Exit(1)}
 fmt.Printf("A3_3_CATALOG_PIN=PASS VERSION=%s BYTES=%d SHA256=%s ORIGIN=%s COMPATIBILITY=%s\n",
  ref.Version,catalogpin.CatalogSize,ref.SHA256,ref.Origin,ref.Compatibility)
}
