// diagnose is an A3.2 test/CI-only, no-server Windows x64 JSON exporter.
// A verified local catalog is required by canonical diagnostic v0.1.0.
package main

import (
 "context"
 "errors"
 "flag"
 "fmt"
 "os"
 "path/filepath"
 "runtime"
 "time"

 "github.com/Herurg123/neurometr-agent/internal/diagnostic"
)

func main(){
 root:=flag.String("root",".","Agent source root with pinned contracts")
 catalog:=flag.String("catalog","","absolute path to an explicitly supplied local v0.1.0 catalog")
 output:=flag.String("output","","private destination for the fully validated diagnostic JSON")
 exe:=flag.String("llmfit","","optional absolute llmfit v1.1.20 executable; not needed for hardware")
 sha:=flag.String("llmfit-sha256","","optional expected SHA-256 for enabled llmfit")
 fixture:=flag.Bool("test-only-catalog",false,"CI ONLY: allow clearly marked synthetic catalog fixtures")
 flag.Parse()
 if runtime.GOOS!="windows"||runtime.GOARCH!="amd64"{
  fmt.Fprintln(os.Stderr,"A3.2 native export is only supported on Windows x64")
  os.Exit(2)
 }
 if *catalog==""||*output==""{
  fmt.Fprintln(os.Stderr,"A3.2 requires -catalog and -output; no catalog is invented")
  os.Exit(2)
 }
 absRoot,err:=filepath.Abs(*root);if err!=nil{os.Exit(2)}
 absCatalog,err:=filepath.Abs(*catalog);if err!=nil{os.Exit(2)}
 absOutput,err:=filepath.Abs(*output);if err!=nil{os.Exit(2)}
 if *exe!="" && !filepath.IsAbs(*exe){fmt.Fprintln(os.Stderr,"llmfit path must be absolute");os.Exit(2)}
 start:=time.Now()
 res,err:=diagnostic.Generate(context.Background(),diagnostic.Options{
  ContractRoot:absRoot,CatalogPath:absCatalog,AllowTestCatalog:*fixture,
  LLMFitExecutable:*exe,LLMFitSHA256:*sha,
 },diagnostic.NativeProbes())
 if err!=nil{
  if errors.Is(err,diagnostic.ErrCatalogRequired){
   fmt.Fprintln(os.Stderr,"CATALOG_BLOCKER: an approved local production catalog is not configured")
  }else{
   fmt.Fprintln(os.Stderr,"A3.2 validation/collection error:",err)
  }
  os.Exit(1)
 }
 if _,err=os.Stat(absOutput);err==nil{
  fmt.Fprintln(os.Stderr,"destination already exists; refusing overwrite")
  os.Exit(2)
 }else if !os.IsNotExist(err){os.Exit(2)}
 f,err:=os.CreateTemp(filepath.Dir(absOutput),".neurometr-private-*.json")
 if err!=nil{fmt.Fprintln(os.Stderr,"private report destination unavailable");os.Exit(2)}
 tmp:=f.Name()
 defer os.Remove(tmp)
 if err=f.Chmod(0600);err==nil{_,err=f.Write(res.JSON)}
 if err==nil{err=f.Sync()}
 closeErr:=f.Close()
 if err!=nil||closeErr!=nil{fmt.Fprintln(os.Stderr,"private report write failed");os.Exit(2)}
 if err=os.Rename(tmp,absOutput);err!=nil{fmt.Fprintln(os.Stderr,"private report save failed");os.Exit(2)}
 // Never print host-, user-, OS-, CPU- or GPU-identifying content in CI logs.
 fmt.Printf("A3_2_REPORT_VALID=true HARDWARE_STATUS=%s LLMFIT_STATUS=%s HARDWARE_MS=%d TOTAL_MS=%d JSON_BYTES=%d\n",
  res.HardwareStatus,res.LLMFitStatus,res.HardwareElapsed.Milliseconds(),time.Since(start).Milliseconds(),len(res.JSON))
}
