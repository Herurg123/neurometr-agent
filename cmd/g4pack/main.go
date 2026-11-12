// g4pack prepares and verifies G4.1 private CI-only Windows ZIPs.
// No release, artifact upload or remote download is performed by this tool.
package main

import (
 "flag"
 "fmt"
 "os"
 "path/filepath"

 "github.com/Herurg123/neurometr-agent/internal/g4bundle"
)
func die(message string) {
 fmt.Fprintln(os.Stderr,"G4_1_BUNDLE=FAIL: "+message)
 os.Exit(2)
}
func main(){
 if len(os.Args)<2{die("use pack or verify")}
 switch os.Args[1] {
 case "pack":
  cmd:=flag.NewFlagSet("pack",flag.ExitOnError)
  root:=cmd.String("root",".","checkout containing canonical pin and approved bundled catalog")
  exe:=cmd.String("exe","","Windows x64 Go PE")
  asset:=cmd.String("official-archive","","pinned original official llmfit v1.1.20 ZIP")
  output:=cmd.String("out","","private CI output ZIP path, not a release artifact")
  sha:=cmd.String("source-commit","","40-hex Agent PR HEAD")
  _=cmd.Parse(os.Args[2:])
  if *exe==""||*asset==""||*output==""||*sha==""{die("missing required build arguments")}
  result,err:=g4bundle.Pack(g4bundle.Config{
   SourceRoot:*root,AgentExe:*exe,OfficialLLMFitArchive:*asset,OutputZIP:*output,SourceCommit:*sha,
  })
  if err!=nil{die("identity, inputs, licenses or packaging validation failed")}
  fmt.Printf("G4_1_PACK=PASS FILES=%d CATALOG=%s LLMFIT_VERSION=%s STATUS=%s\n",
    len(result.Files),result.CatalogVersion,result.LLMFitVersion,result.Distribution)
 case "verify":
  cmd:=flag.NewFlagSet("verify",flag.ExitOnError)
  input:=cmd.String("zip","","private CI package ZIP")
  dest:=cmd.String("dest","","optional new empty extraction folder")
  _=cmd.Parse(os.Args[2:])
  if *input==""{die("missing ZIP path")}
  if *dest!=""&&!filepath.IsAbs(*dest){die("extraction path must be absolute")}
  m,err:=g4bundle.Verify(*input,*dest)
  if err!=nil{die("ZIP integrity, inventory or path validation failed")}
  fmt.Printf("G4_1_VERIFY=PASS FILES=%d CATALOG=%s ORIGIN=APPROVED_BUNDLED LLMFIT_ARCHIVE_SHA256=%s\n",
    len(m.Files),m.CatalogVersion,m.LLMFitArchiveSHA256)
 default:
  die("unsupported operation")
 }
}
