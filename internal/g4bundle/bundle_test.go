package g4bundle

import (
 "archive/zip"
 "bytes"
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "sort"
 "strings"
 "testing"
 "time"
)

func TestUnsafePathsRejected(t *testing.T){
 for _,name:=range []string{"", ".", "..", "../etc", "contracts/../bin/bad", "/absolute", "C:/windows", "a\\b",
  "a//b","../.env","one/./two","folder.","folder ","x/\x00bad","a\nb","a\r\nb"}{
  if validName(name)==nil{t.Fatalf("unsafe ZIP path accepted: %q",name)}
 }
 for _,name:=range []string{"Neurometr.exe","bin/llmfit.exe","contracts/fixtures/F01_diagnostic_windows_ok.json","LICENSES/llmfit-MIT.txt"}{
  if validName(name)!=nil{t.Fatalf("ordinary package path rejected: %q",name)}
 }
}
func TestOfficialCLIFailsClosedOnMissingOrWrongPin(t *testing.T){
 if _,err:=OfficialCLI(filepath.Join(t.TempDir(),"missing.zip"));!errors.Is(err,ErrInvalidBundle){t.Fatalf("missing archive accepted: %v",err)}
 wrong:=filepath.Join(t.TempDir(),"wrong.zip")
 if err:=os.WriteFile(wrong,bytes.Repeat([]byte{0},OfficialArchiveBytes),0600);err!=nil{t.Fatal(err)}
 if _,err:=OfficialCLI(wrong);!errors.Is(err,ErrInvalidBundle){t.Fatalf("same-length wrong SHA archive accepted: %v",err)}
}
type testBundle struct {
 files map[string][]byte
 manifest Manifest
}
func makeSyntheticPackage(t *testing.T)testBundle{
 t.Helper()
 files:=map[string][]byte{
  "Neurometr.exe":[]byte("MZsynthetic-cli-not-executable"),
  "bin/llmfit.exe":[]byte("MZsynthetic-llmfit-not-executable"),
  "contracts/contract.lock.json":[]byte("{}"),
  "contracts/diagnostic.schema.json":[]byte("{}"),
  "bundled/catalog.json":bytes.Repeat([]byte("a"),catalogSizeForTest()),
  "bundled/catalog.lock.json":[]byte("{}"),
  "LICENSES/llmfit-MIT.txt":[]byte("MIT full source copy test"),
  "LICENSES/jsonschema-Apache-2.0.txt":[]byte("Apache full source copy test"),
  "LICENSES/golang-x-text-BSD-3-Clause.txt":[]byte("Go x/text license test"),
  "LICENSES/go-toolchain-BSD-3-Clause.txt":[]byte("Go toolchain license test"),
  "THIRD_PARTY_NOTICES_RU.txt":[]byte("Notices test"),
  "README_RU.txt":[]byte("Readme test"),
 }
 manifest:=Manifest{
  Format:ManifestFormat,Distribution:"internal-ci-only-not-for-publication",
  SourceCommit:strings.Repeat("a",40),ContractVersion:"0.1.0",
  CatalogVersion:"2026.11.11.2",CatalogSHA256:"1d84ff596c589a4db004f8a3fa17490d5479f57db3d7b0a433a7e448289651d2",
  LLMFitVersion:LLMBinaryVersion,LLMFitArchiveSHA256:OfficialArchiveSHA256,
  LLMFitBinarySHA256:hexSHA(files["bin/llmfit.exe"]),
 }
 for name,data:=range files{
  manifest.Files=append(manifest.Files,FileEntry{Path:name,Bytes:int64(len(data)),SHA256:hexSHA(data)})
 }
 sort.Slice(manifest.Files,func(i,j int)bool{return manifest.Files[i].Path<manifest.Files[j].Path})
 return testBundle{files:files,manifest:manifest}
}
func catalogSizeForTest()int{return 6589}
func zipSynthetic(t *testing.T,files map[string][]byte,paths []string)string{
 t.Helper()
 file:=filepath.Join(t.TempDir(),"candidate.zip")
 out,err:=os.Create(file);if err!=nil{t.Fatal(err)}
 zw:=zip.NewWriter(out)
 for _,name:=range paths{
  if err=appendZipEntry(zw,name,files[name]);err!=nil{t.Fatal(err)}
 }
 if err=zw.Close();err!=nil{t.Fatal(err)}
 if err=out.Close();err!=nil{t.Fatal(err)}
 return file
}
func TestManifestSizeAndIdentityControls(t *testing.T){
 b:=makeSyntheticPackage(t)
 // This is intentionally invalid because its fictional catalog has
 // the approved size but NOT the approved SHA-256.
 manifestRaw,err:=json.Marshal(b.manifest);if err!=nil{t.Fatal(err)}
 b.files[ManifestPath]=manifestRaw
 paths:=make([]string,0,len(b.files));for p:=range b.files{paths=append(paths,p)};sort.Strings(paths)
 archive:=zipSynthetic(t,b.files,paths)
 if _,err=Verify(archive,"");!errors.Is(err,ErrInvalidBundle){t.Fatal("fake catalog passed exact approved SHA-256 pin")}
}
func TestZIPTraversalAndCaseAliasFailBeforeExtraction(t *testing.T){
 b:=makeSyntheticPackage(t)
 data,_:=json.Marshal(b.manifest)
 b.files[ManifestPath]=data
 cases:=[][]string{
  {"/abs"},
  {"../outside.json"},
  {"contracts/../../../secret"},
  {"bin/llmfit.exe","bin/LLMFIT.EXE"},
  {"C:/drive"},
  {"some\\windows-path"},
 }
 for _,names:=range cases{
  t.Run(strings.ReplaceAll(strings.Join(names,"_"),"/","-"),func(t *testing.T){
   pathEntries:=append([]string{},names...)
   // Entries are invalid even if they appear in an otherwise plausible ZIP.
   zz:=zipSynthetic(t,b.files,pathEntries)
   dest:=filepath.Join(t.TempDir(),"should-not-exist")
   if _,err:=Verify(zz,dest);!errors.Is(err,ErrInvalidBundle){t.Fatalf("unsafe package accepted: %v",err)}
   if _,err:=os.Lstat(dest);!errors.Is(err,os.ErrNotExist){t.Fatal("untrusted ZIP modified extraction directory")}
  })
 }
}
func TestZIPWriterIsDeterministic(t *testing.T){
 b:=[]byte("G4 deterministic unit")
 pathName:="README_RU.txt"
 makeZip:=func()[]byte{
  var buf bytes.Buffer
  writer:=zip.NewWriter(&buf)
  if err:=appendZipEntry(writer,pathName,b);err!=nil{t.Fatal(err)}
  if err:=writer.Close();err!=nil{t.Fatal(err)}
  return buf.Bytes()
 }
 a,c:=makeZip(),makeZip()
 if !bytes.Equal(a,c){t.Fatal("timestamp or ZIP compressor nondeterminism")}
 r,err:=zip.NewReader(bytes.NewReader(a),int64(len(a)))
 if err!=nil||len(r.File)!=1{t.Fatal("invalid reproducible ZIP")}
 if !r.File[0].Modified.Equal(time.Date(1980,1,1,0,0,0,0,time.UTC)){
  t.Fatalf("nonreproducible time: %s",r.File[0].Modified)
 }
}
