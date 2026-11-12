// Package g4bundle builds and validates a TEST-DISTRIBUTION, deterministic, offline
// Windows x64 ZIP. It does not publish, execute, download, or sign anything.
// Its manifest is an inventory, not a publisher authentication primitive.
package g4bundle

import (
 "archive/zip"
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "os"
 "path"
 "path/filepath"
 "regexp"
 "sort"
 "strings"
 "time"

 "github.com/Herurg123/neurometr-agent/internal/catalogpin"
 "github.com/Herurg123/neurometr-agent/internal/contract"
)

const (
 PackageRoot = "Neurometr-Windows-x64"
 ManifestPath = "release-manifest.json"
 ManifestFormat = "neurometr.g4.2.test-inventory"
 LLMBinaryVersion = "1.1.20"
 OfficialArchiveName = "llmfit-v1.1.20-x86_64-pc-windows-msvc.zip"
 OfficialArchiveBytes = 6878103
 OfficialArchiveSHA256 = "1f848ddf2554adb89b3a31a034245826694148249fadace572c4544bb3c9c489"
 maxPackageBytes = 190 * 1024 * 1024
 maxArchiveExtracted = 128 * 1024 * 1024
 maxPackageFiles = 100
 maxRegularFile = 128 * 1024 * 1024
)

var (
 ErrInvalidBundle = errors.New("G4.1 internal package integrity failure")
 gitCommit=regexp.MustCompile("^[0-9a-f]{40}$")
)
type Config struct {
 SourceRoot string
 AgentExe string
 OfficialLLMFitArchive string
 OutputZIP string
 SourceCommit string
}
type FileEntry struct {
 Path string `json:"path"`
 Bytes int64 `json:"bytes"`
 SHA256 string `json:"sha256"`
}
type Manifest struct {
 Format string `json:"format"`
 Distribution string `json:"distribution"`
 SourceCommit string `json:"agent_source_commit"`
 ContractVersion string `json:"diagnostic_contract_version"`
 CatalogVersion string `json:"catalog_version"`
 CatalogSHA256 string `json:"catalog_sha256"`
 LLMFitVersion string `json:"llmfit_version"`
 LLMFitArchiveSHA256 string `json:"llmfit_official_archive_sha256"`
 LLMFitBinarySHA256 string `json:"llmfit_binary_sha256"`
 Files []FileEntry `json:"files"`
}
func hexSHA(b []byte) string {s:=sha256.Sum256(b);return hex.EncodeToString(s[:])}
func validName(name string) error {
 if name==""||name=="."||strings.HasPrefix(name,"/")||strings.ContainsAny(name,"\\:\r\n\x00")||
    path.Clean(name)!=name||strings.Contains(name,"//")||len(name)>220{
  return ErrInvalidBundle
 }
 for _,p:=range strings.Split(name,"/"){
  if p=="."||p==".."||p==""||strings.HasSuffix(p,".")||strings.HasSuffix(p," "){
   return ErrInvalidBundle
  }
 }
 return nil
}
func readRegular(file string,limit int64)([]byte,error){
 st,e:=os.Lstat(file)
 if e!=nil||!st.Mode().IsRegular()||st.Size()<=0||st.Size()>limit{return nil,ErrInvalidBundle}
 b,e:=os.ReadFile(file)
 if e!=nil||int64(len(b))!=st.Size(){return nil,ErrInvalidBundle}
 return b,nil
}
func readZipPart(f *zip.File,max int64)([]byte,error){
 if !f.Mode().IsRegular()||f.UncompressedSize64==0||f.UncompressedSize64>uint64(max){return nil,ErrInvalidBundle}
 r,e:=f.Open();if e!=nil{return nil,ErrInvalidBundle}
 defer r.Close()
 b,e:=io.ReadAll(io.LimitReader(r,max+1))
 if e!=nil||int64(len(b))!=int64(f.UncompressedSize64)||int64(len(b))>max{
  return nil,ErrInvalidBundle
 }
 return b,nil
}
// OfficialCLI requires the EXACT owner-approved archive checksum and length.
// Only one bounded Windows PE is extracted in memory, never run here.
func OfficialCLI(archivePath string)([]byte,error){
 b,e:=readRegular(archivePath,OfficialArchiveBytes)
 if e!=nil||len(b)!=OfficialArchiveBytes||hexSHA(b)!=OfficialArchiveSHA256{return nil,ErrInvalidBundle}
 z,e:=zip.NewReader(bytes.NewReader(b),int64(len(b)))
 if e!=nil||len(z.File)==0||len(z.File)>30{return nil,ErrInvalidBundle}
 seen:=map[string]bool{}
 var total uint64
 var binary []byte
 for _,f:=range z.File{
  n:=strings.TrimSuffix(f.Name,"/")
  if e=validName(n);e!=nil{return nil,ErrInvalidBundle}
  low:=strings.ToLower(n)
  if seen[low]{return nil,ErrInvalidBundle}
  seen[low]=true
  total+=f.UncompressedSize64
  if total>maxArchiveExtracted{return nil,ErrInvalidBundle}
  if strings.HasSuffix(f.Name,"/"){
   if !f.Mode().IsDir(){return nil,ErrInvalidBundle}
   continue
  }
  if !f.Mode().IsRegular()||f.UncompressedSize64>maxRegularFile{return nil,ErrInvalidBundle}
  if strings.EqualFold(path.Base(n),"llmfit.exe"){
   if binary!=nil{return nil,ErrInvalidBundle}
   binary,e=readZipPart(f,maxRegularFile);if e!=nil{return nil,e}
  }
 }
 if len(binary)<2||!bytes.Equal(binary[:2],[]byte("MZ")){return nil,ErrInvalidBundle}
 return binary,nil
}
func readContractFiles(root string)(map[string][]byte,error){
 dir:=filepath.Join(root,"contracts")
 entries:=map[string][]byte{}
 err:=filepath.WalkDir(dir,func(p string,d os.DirEntry,walkErr error)error{
  if walkErr!=nil{return ErrInvalidBundle}
  if d.Type()&os.ModeSymlink!=0{return ErrInvalidBundle}
  if d.IsDir(){return nil}
  if !d.Type().IsRegular(){return ErrInvalidBundle}
  rel,e:=filepath.Rel(root,p);if e!=nil{return ErrInvalidBundle}
  n:=filepath.ToSlash(rel)
  if validName(n)!=nil{return ErrInvalidBundle}
  b,e:=readRegular(p,2*1024*1024)
  if e!=nil{return ErrInvalidBundle}
  entries[n]=b
  if len(entries)>40{return ErrInvalidBundle}
  return nil
 })
 if err!=nil||len(entries)<23{return nil,ErrInvalidBundle}
 return entries,nil
}
func packageInputs(c Config)(map[string][]byte,Manifest,error){
 zero:=Manifest{}
 if !gitCommit.MatchString(c.SourceCommit){return nil,zero,ErrInvalidBundle}
 root,e:=filepath.Abs(c.SourceRoot);if e!=nil{return nil,zero,ErrInvalidBundle}
 v,e:=contract.NewFullValidator(root);if e!=nil{return nil,zero,fmt.Errorf("%w: contracts pin or schema",ErrInvalidBundle)}
 catalog,e:=catalogpin.Verify(root,v)
 if e!=nil||catalog.Origin!="bundled"||catalog.Version!=catalogpin.CatalogVersion||catalog.SHA256!=catalogpin.CatalogSHA256{
  return nil,zero,fmt.Errorf("%w: owner-approved catalog",ErrInvalidBundle)
 }
 contracts,e:=readContractFiles(root);if e!=nil{return nil,zero,e}
 binary,e:=readRegular(c.AgentExe,maxRegularFile)
 if e!=nil||len(binary)<2||string(binary[:2])!="MZ"{return nil,zero,fmt.Errorf("%w: Agent must be a Windows PE",ErrInvalidBundle)}
 cli,e:=OfficialCLI(c.OfficialLLMFitArchive)
 if e!=nil{return nil,zero,fmt.Errorf("%w: pinned official llmfit archive",ErrInvalidBundle)}
 files:=map[string][]byte{
  "Neurometr.exe":binary,
  "bin/llmfit.exe":cli,
 }
 for name,data:=range contracts{files[name]=data}
 sourcePaths:=map[string]string{
  "bundled/catalog.json":"bundled/catalog.json",
  "bundled/catalog.lock.json":"bundled/catalog.lock.json",
  "LICENSES/llmfit-MIT.txt":"packaging/licenses/llmfit-MIT.txt",
  "LICENSES/react-family-MIT.txt":"packaging/licenses/react-family-MIT.txt",
  "LICENSES/rust-cli-source-license-texts.txt":"packaging/licenses/rust-cli-source-license-texts.txt",
  "LICENSES/clipboard-win-BSL-1.0.txt":"packaging/licenses/clipboard-win-BSL-1.0.txt",
  "LICENSES/rmcp-3.5.1-upstream-LICENSE.txt":"packaging/licenses/rmcp-3.5.1-upstream-LICENSE.txt",
  "LICENSES/js-tokens-MIT.txt":"packaging/licenses/js-tokens-MIT.txt",
  "LICENSES/loose-envify-MIT.txt":"packaging/licenses/loose-envify-MIT.txt",
  "DISTRIBUTION_TERMS_RU.txt":"packaging/DISTRIBUTION_TERMS_RU.txt",
  "DATA_SOURCE_ATTRIBUTION_RU.txt":"packaging/DATA_SOURCE_ATTRIBUTION_RU.txt",
  "LLMFIT_WINDOWS_CLI_AUDIT_RU.txt":"packaging/LLMFIT_WINDOWS_CLI_AUDIT_RU.txt",
  "LLMFIT_WINDOWS_CLI_COMPONENTS_RU.txt":"packaging/LLMFIT_WINDOWS_CLI_COMPONENTS_RU.txt",
  "LICENSES/jsonschema-Apache-2.0.txt":"packaging/licenses/jsonschema-Apache-2.0.txt",
  "LICENSES/golang-x-text-BSD-3-Clause.txt":"packaging/licenses/golang-x-text-BSD-3-Clause.txt",
  "LICENSES/go-toolchain-BSD-3-Clause.txt":"packaging/licenses/go-toolchain-BSD-3-Clause.txt",
  "THIRD_PARTY_NOTICES_RU.txt":"packaging/THIRD_PARTY_NOTICES_RU.txt",
  "README_RU.txt":"packaging/README_RU.txt",
 }
 for target,source:=range sourcePaths{
  docMax:=int64(2*1024*1024)
  if target=="LICENSES/rust-cli-source-license-texts.txt"{docMax=16*1024*1024}
  b,err:=readRegular(filepath.Join(root,filepath.FromSlash(source)),docMax)
  if err!=nil{return nil,zero,fmt.Errorf("%w: missing pinned distribution document",ErrInvalidBundle)}
  files[target]=b
 }
 if len(files)>maxPackageFiles{return nil,zero,ErrInvalidBundle}
 manifest:=Manifest{
  Format:ManifestFormat,
  Distribution:"user-qa-distribution",
  SourceCommit:c.SourceCommit,
  ContractVersion:"0.1.0",
  CatalogVersion:catalogpin.CatalogVersion,
  CatalogSHA256:catalogpin.CatalogSHA256,
  LLMFitVersion:LLMBinaryVersion,
  LLMFitArchiveSHA256:OfficialArchiveSHA256,
  LLMFitBinarySHA256:hexSHA(cli),
  Files:make([]FileEntry,0,len(files)),
 }
 for n,b:=range files{
  if validName(n)!=nil{return nil,zero,ErrInvalidBundle}
  manifest.Files=append(manifest.Files,FileEntry{Path:n,Bytes:int64(len(b)),SHA256:hexSHA(b)})
 }
 sort.Slice(manifest.Files,func(i,j int)bool{return manifest.Files[i].Path<manifest.Files[j].Path})
 return files,manifest,nil
}
func appendZipEntry(z *zip.Writer,name string,b []byte)error{
 h:=&zip.FileHeader{Name:PackageRoot+"/"+name,Method:zip.Deflate}
 h.Modified=time.Date(1980,1,1,0,0,0,0,time.UTC)
 h.SetMode(0644)
 w,e:=z.CreateHeader(h);if e!=nil{return e}
 _,e=w.Write(b);return e
}
// Pack writes only to an explicit private CI path, with fixed ordering,
// timestamps and compression. Manifest does not include itself, avoiding an
// impossible self-hash and does not authenticate the publisher.
func Pack(c Config)(Manifest,error){
 files,m,e:=packageInputs(c);if e!=nil{return Manifest{},e}
 manifestRaw,e:=json.MarshalIndent(m,"","  ");if e!=nil{return Manifest{},ErrInvalidBundle}
 manifestRaw=append(manifestRaw,'\n')
 output,e:=filepath.Abs(c.OutputZIP)
 if e!=nil||strings.EqualFold(output,filepath.Join(c.SourceRoot,"")){return Manifest{},ErrInvalidBundle}
 if _,statErr:=os.Lstat(output);statErr==nil{return Manifest{},ErrInvalidBundle}else if !errors.Is(statErr,os.ErrNotExist){return Manifest{},ErrInvalidBundle}
 dir:=filepath.Dir(output)
 info,e:=os.Lstat(dir);if e!=nil||!info.IsDir()||info.Mode()&os.ModeSymlink!=0{return Manifest{},ErrInvalidBundle}
 tmp,e:=os.CreateTemp(dir,".g4-test-*.zip.tmp");if e!=nil{return Manifest{},e}
 tmpPath:=tmp.Name();defer os.Remove(tmpPath)
 z:=zip.NewWriter(tmp)
 for _,f:=range m.Files{
  if e=appendZipEntry(z,f.Path,files[f.Path]);e!=nil{break}
 }
 if e==nil{e=appendZipEntry(z,ManifestPath,manifestRaw)}
 closeErr:=z.Close()
 if e==nil{e=closeErr}
 if e==nil{e=tmp.Sync()}
 fileErr:=tmp.Close()
 if e==nil{e=fileErr}
 if e!=nil{return Manifest{},ErrInvalidBundle}
 if e=os.Rename(tmpPath,output);e!=nil{return Manifest{},ErrInvalidBundle}
 return m,nil
}
// Verify checks ZIP traversal, regular-file requirements, case-insensitive
// duplicate names, bounded lengths, exact manifest inventory and digests.
// Extract is optional and starts only AFTER every byte has passed validation.
func Verify(zipPath,dest string)(Manifest,error){
 zero:=Manifest{}
 b,e:=readRegular(zipPath,maxPackageBytes)
 if e!=nil{return zero,ErrInvalidBundle}
 reader,e:=zip.NewReader(bytes.NewReader(b),int64(len(b)))
 if e!=nil||len(reader.File)<25||len(reader.File)>maxPackageFiles+1{return zero,ErrInvalidBundle}
 files:=map[string][]byte{}
 seen:=map[string]bool{}
 var total uint64
 for _,f:=range reader.File{
  full:=f.Name
  if !strings.HasPrefix(full,PackageRoot+"/"){return zero,ErrInvalidBundle}
  n:=strings.TrimPrefix(full,PackageRoot+"/")
  if validName(n)!=nil{return zero,ErrInvalidBundle}
  folded:=strings.ToLower(n)
  if seen[folded]{return zero,ErrInvalidBundle}
  seen[folded]=true
  if f.UncompressedSize64>maxRegularFile{return zero,ErrInvalidBundle}
  total+=f.UncompressedSize64
  if total>maxPackageBytes{return zero,ErrInvalidBundle}
  content,e:=readZipPart(f,maxRegularFile);if e!=nil{return zero,ErrInvalidBundle}
  files[n]=content
 }
 raw,ok:=files[ManifestPath]
 if !ok||len(raw)>1024*1024{return zero,ErrInvalidBundle}
 var m Manifest
 dec:=json.NewDecoder(bytes.NewReader(raw));dec.DisallowUnknownFields()
 if e=dec.Decode(&m);e!=nil{return zero,ErrInvalidBundle}
 var tail any
 if dec.Decode(&tail)!=io.EOF{return zero,ErrInvalidBundle}
 if m.Format!=ManifestFormat||m.Distribution!="user-qa-distribution"||!gitCommit.MatchString(m.SourceCommit)||
  m.ContractVersion!="0.1.0"||m.CatalogVersion!=catalogpin.CatalogVersion||
  m.CatalogSHA256!=catalogpin.CatalogSHA256||m.LLMFitVersion!=LLMBinaryVersion||
  m.LLMFitArchiveSHA256!=OfficialArchiveSHA256||len(m.Files)!=len(files)-1{
   return zero,ErrInvalidBundle
 }
 required:=[]string{"Neurometr.exe","bin/llmfit.exe","bundled/catalog.json","bundled/catalog.lock.json",
  "contracts/contract.lock.json","contracts/diagnostic.schema.json","LICENSES/llmfit-MIT.txt",
  "LICENSES/jsonschema-Apache-2.0.txt","LICENSES/golang-x-text-BSD-3-Clause.txt",
  "LICENSES/go-toolchain-BSD-3-Clause.txt","LICENSES/react-family-MIT.txt",
  "LICENSES/js-tokens-MIT.txt","LICENSES/loose-envify-MIT.txt",
  "LICENSES/rust-cli-source-license-texts.txt",
  "LICENSES/clipboard-win-BSL-1.0.txt","LICENSES/rmcp-3.5.1-upstream-LICENSE.txt",
  "DISTRIBUTION_TERMS_RU.txt","DATA_SOURCE_ATTRIBUTION_RU.txt",
  "LLMFIT_WINDOWS_CLI_AUDIT_RU.txt","LLMFIT_WINDOWS_CLI_COMPONENTS_RU.txt","THIRD_PARTY_NOTICES_RU.txt","README_RU.txt"}
 for _,p:=range required{if len(files[p])==0{return zero,ErrInvalidBundle}}
 if len(files["bundled/catalog.json"])!=catalogpin.CatalogSize||hexSHA(files["bundled/catalog.json"])!=catalogpin.CatalogSHA256{
  return zero,ErrInvalidBundle
 }
 if hexSHA(files["bin/llmfit.exe"])!=m.LLMFitBinarySHA256||
  len(files["bin/llmfit.exe"])<2||string(files["bin/llmfit.exe"][:2])!="MZ"||
  len(files["Neurometr.exe"])<2||string(files["Neurometr.exe"][:2])!="MZ"{
  return zero,ErrInvalidBundle
 }
 known:=map[string]bool{}
 prev:=""
 for _,it:=range m.Files{
  if validName(it.Path)!=nil||it.Path<=prev||known[strings.ToLower(it.Path)]||it.Path==ManifestPath{
   return zero,ErrInvalidBundle
  }
  prev=it.Path;known[strings.ToLower(it.Path)]=true
  raw,ok:=files[it.Path]
  if !ok||it.Bytes<=0||int64(len(raw))!=it.Bytes||hexSHA(raw)!=it.SHA256{
   return zero,ErrInvalidBundle
  }
 }
 if dest==""{return m,nil}
 target,e:=filepath.Abs(dest);if e!=nil{return zero,ErrInvalidBundle}
 if _,e=os.Lstat(target);!errors.Is(e,os.ErrNotExist){return zero,ErrInvalidBundle}
 if err:=os.Mkdir(target,0700);err!=nil{return zero,ErrInvalidBundle}
 // Do not follow arbitrary pre-existing dirs: the destination was absent.
 for _,it:=range m.Files{
  filename:=filepath.Join(target,filepath.FromSlash(it.Path))
  if err:=os.MkdirAll(filepath.Dir(filename),0700);err!=nil{return zero,ErrInvalidBundle}
  if err:=os.WriteFile(filename,files[it.Path],0600);err!=nil{return zero,ErrInvalidBundle}
 }
 if err:=os.WriteFile(filepath.Join(target,ManifestPath),raw,0600);err!=nil{return zero,ErrInvalidBundle}
 return m,nil
}
