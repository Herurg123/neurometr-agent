// Package contract provides the A1 portable numeric, pin and targeted semantic checks.
// This is NOT a complete JSON Schema Draft 2020-12 implementation.
// The pinned schemas remain normative; full independent schema validation is an A2+ dependency.
package contract

import (
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "math"
 "math/big"
 "os"
 "path/filepath"
 "reflect"
 "regexp"
 "sort"
 "strconv"
 "strings"
 "time"
 "unicode/utf8"
)

const SourceCommit = "97aa0f4c1ceaef36608288e589b8abe935c5189e"
const SourceRepo = "Herurg123/neurometr-web"
const Version = "0.1.0"
const DiagnosticLimit = 2 * 1024 * 1024
const CatalogLimit = 20 * 1024 * 1024
const maxDepth = 64
const maxNodes = 100000
const safeInteger int64 = 9007199254740991

type PinFile struct {
 Path string `json:"path"`
 SHA256 string `json:"sha256"`
 Bytes int `json:"bytes"`
}
type PinLock struct {
 ContractVersion string `json:"contract_version"`
 Source struct {
  Repository string `json:"repository"`
  Commit string `json:"commit"`
  PathPrefix string `json:"path_prefix"`
 } `json:"source"`
 Algorithm string `json:"algorithm"`
 Files []PinFile `json:"files"`
}

// VerifyPin checks original bytes; Git blob SHA must never be used as SHA-256.
func VerifyPin(root string) error {
 raw,err:=os.ReadFile(filepath.Join(root,"contracts","contract.lock.json"));if err!=nil{return err}
 var lock PinLock
 if err=json.Unmarshal(raw,&lock);err!=nil{return err}
 if lock.Source.Repository!=SourceRepo||lock.Source.Commit!=SourceCommit||lock.Source.PathPrefix!="contracts/"||lock.ContractVersion!=Version||lock.Algorithm!="SHA-256" {return errors.New("contract lock source/version mismatch")}
 if len(lock.Files)!=23{return fmt.Errorf("expected 23 canonical files, got %d",len(lock.Files))}
 seen:=map[string]bool{}
 for _,f:=range lock.Files {
  if !strings.HasPrefix(f.Path,"contracts/")||strings.Contains(f.Path,"..")||strings.Contains(f.Path,"\")||f.Path=="contracts/contract.lock.json"||seen[f.Path]{return fmt.Errorf("invalid or duplicate pinned path: %s",f.Path)}
  seen[f.Path]=true
  b,err:=os.ReadFile(filepath.Join(root,filepath.FromSlash(f.Path)));if err!=nil{return err}
  if len(b)!=f.Bytes||hex.EncodeToString(sha256sum(b))!=f.SHA256{return fmt.Errorf("pinned file changed: %s",f.Path)}
 }
 err=filepath.WalkDir(filepath.Join(root,"contracts"),func(path string,d os.DirEntry,err error)error{
  if err!=nil{return err};if d.IsDir(){return nil}
  relative,err:=filepath.Rel(root,path);if err!=nil{return err}
  relative=filepath.ToSlash(relative)
  if relative!="contracts/contract.lock.json"&&!seen[relative]{return fmt.Errorf("unlisted canonical file: %s",relative)}
  return nil
 })
 return err
}
func sha256sum(b []byte) []byte { v:=sha256.Sum256(b); return v[:] }

// VerifySchemaHeaders checks identity/draft, not full metaschema compliance.
func VerifySchemaHeaders(root string)error{
 for name,id:=range map[string]string{
  "diagnostic.schema.json":"diagnostic",
  "catalog.schema.json":"catalog",
  "catalog-manifest.schema.json":"catalog-manifest",
  "latest-pointer.schema.json":"catalog-pointer",
 } {
  b,err:=os.ReadFile(filepath.Join(root,"contracts",name));if err!=nil{return err}
  var m map[string]any;if err=json.Unmarshal(b,&m);err!=nil{return err}
  if m["$schema"]!="https://json-schema.org/draft/2020-12/schema"||m["$id"]!="urn:neurometr:schema:"+id+":0.1.0"||m["type"]!="object"{return fmt.Errorf("schema header mismatch: %s",name)}
 }
 return nil
}

var numberPattern=regexp.MustCompile("^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$")
func decimalRat(token string)(*big.Rat,error){
 if !numberPattern.MatchString(token){return nil,errors.New("invalid number")}
 s:=strings.ToLower(token)
 expo:=0
 if i:=strings.IndexByte(s,'e');i>=0{
  var err error;expo,err=strconv.Atoi(s[i+1:]);if err!=nil||expo>10000||expo< -10000{return nil,errors.New("numeric exponent too large")}
  s=s[:i]
 }
 sign:=1;if strings.HasPrefix(s,"-"){sign=-1;s=s[1:]}
 frac:=0;if i:=strings.IndexByte(s,'.');i>=0{frac=len(s)-i-1;s=s[:i]+s[i+1:]}
 n,ok:=new(big.Int).SetString(s,10);if !ok{return nil,errors.New("invalid digits")}
 if sign<0 {n.Neg(n)}
 shift:=expo-frac
 rat:=new(big.Rat).SetInt(n)
 if shift>0 {rat.Mul(rat,new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10),big.NewInt(int64(shift)),nil)))}
 if shift<0 {rat.Quo(rat,new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10),big.NewInt(int64(-shift)),nil)))}
 return rat,nil
}

// CheckNumber follows the portable v0.1.0 decimal profile, not Go's lossy default decoding.
func CheckNumber(token string) error {
 r,err:=decimalRat(token);if err!=nil{return err}
 limit:=new(big.Rat).SetInt64(safeInteger)
 if r.IsInt() && (r.Cmp(limit)>0||r.Cmp(new(big.Rat).Neg(limit))<0){return errors.New("unsafe integral JSON number")}
 f,err:=strconv.ParseFloat(token,64)
 if err!=nil||math.IsNaN(f)||math.IsInf(f,0){return errors.New("float overflow")}
 if f==0&&r.Sign()!=0 {return errors.New("float underflow")}
 if math.Trunc(f)==f && math.Abs(f)>float64(safeInteger){return errors.New("unsafe integral float")}
 roundtrip,err:=decimalRat(strconv.FormatFloat(f,'g',-1,64))
 if err!=nil||r.Cmp(roundtrip)!=0{return errors.New("decimal precision loss")}
 return nil
}

// DecodeBounded rejects duplicate keys, invalid UTF-8, nonportable numerics, and excessive depth.
func DecodeBounded(raw []byte, limit int)(any,error){
 if len(raw)>limit{return nil,errors.New("input size limit exceeded")}
 if !utf8.Valid(raw){return nil,errors.New("invalid UTF-8")}
 d:=json.NewDecoder(bytes.NewReader(raw));d.UseNumber();nodes:=0
 v,err:=parseValue(d,1,&nodes);if err!=nil{return nil,err}
 if _,err=d.Token();err!=io.EOF{return nil,errors.New("trailing JSON")}
 return v,nil
}
func parseValue(d *json.Decoder,depth int,nodes *int)(any,error){
 *nodes=*nodes+1
 if depth>maxDepth||*nodes>maxNodes{return nil,errors.New("JSON structural budget exceeded")}
 t,err:=d.Token();if err!=nil{return nil,err}
 switch x:=t.(type){
 case json.Delim:
  switch x {
  case '{':
   m:=map[string]any{}
   for d.More(){
    k,err:=d.Token();if err!=nil{return nil,err}
    key,ok:=k.(string);if !ok{return nil,errors.New("object key not string")}
    if _,found:=m[key];found{return nil,fmt.Errorf("duplicate JSON key: %s",key)}
    v,err:=parseValue(d,depth+1,nodes);if err!=nil{return nil,err};m[key]=v
   }
   if _,err:=d.Token();err!=nil{return nil,err};return m,nil
  case '[':
   a:=[]any{};for d.More(){v,err:=parseValue(d,depth+1,nodes);if err!=nil{return nil,err};a=append(a,v)}
   if _,err:=d.Token();err!=nil{return nil,err};return a,nil
  }
  return nil,errors.New("unexpected delimiter")
 case json.Number:
  if err:=CheckNumber(string(x));err!=nil{return nil,err};return x,nil
 default:return x,nil
 }
}

func object(v any)map[string]any{m,_:=v.(map[string]any);return m}
func array(v any)[]any{a,_:=v.([]any);return a}
func stringOf(v any)string{s,_:=v.(string);return s}
func contains(a []any,s string)bool{for _,v:=range a{if stringOf(v)==s{return true}};return false}
func enum(s string,valid ...string)bool{for _,v:=range valid{if v==s{return true}};return false}
func required(m map[string]any,keys ...string)error{
 if m==nil{return errors.New("expected JSON object")}
 for _,k:=range keys{if _,ok:=m[k];!ok{return fmt.Errorf("missing required field %s",k)}}
 return nil
}
func requireArray(m map[string]any,key string)error{if _,ok:=m[key].([]any);!ok{return fmt.Errorf("%s must be an array",key)};return nil}

// Validate is an A1 targeted semantic validator, not a complete JSON Schema evaluator.
func Validate(kind string, raw []byte)(map[string]any,error){
 limit:=CatalogLimit;if kind=="diagnostic"{limit=DiagnosticLimit}
 v,err:=DecodeBounded(raw,limit);if err!=nil{return nil,err}
 d:=object(v)
 if d==nil||stringOf(d["schema_version"])!=Version{return nil,errors.New("unsupported schema version")}
 expected:=map[string]string{"diagnostic":"neurometr.diagnostic","catalog":"neurometr.catalog","manifest":"neurometr.catalog-manifest","pointer":"neurometr.catalog-pointer"}[kind]
 if expected==""||d["kind"]!=expected{return nil,errors.New("unexpected kind")}
 switch kind {
 case "diagnostic":err=validateDiagnostic(d)
 case "catalog":err=validateCatalog(d)
 case "manifest":err=validateManifest(d)
 case "pointer":err=validatePointer(d)
 }
 if err!=nil{return nil,err}
 return d,nil
}

func validateDiagnostic(d map[string]any)error{
 if err:=required(d,"run_id","started_at","finished_at","overall_status","producer","engines","catalog_ref","hardware","runtime_checks","model_assessments","cloud_checks","components","errors","limitations");err!=nil{return err}
 for _,k:=range []string{"engines","runtime_checks","model_assessments","cloud_checks","components","errors","limitations"}{if err:=requireArray(d,k);err!=nil{return err}}
 if err:=required(object(d["hardware"]),"os","cpu","ram","gpus","npus");err!=nil{return err}
 for _,k:=range []string{"gpus","npus"}{if err:=requireArray(object(d["hardware"]),k);err!=nil{return err}}
 a,b:=stringOf(d["started_at"]),stringOf(d["finished_at"])
 if !strings.HasSuffix(a,"Z")||!strings.HasSuffix(b,"Z"){return errors.New("timestamp must be UTC Z")}
 t1,e1:=time.Parse(time.RFC3339Nano,a);t2,e2:=time.Parse(time.RFC3339Nano,b)
 if e1!=nil||e2!=nil{return errors.New("invalid timestamps")}
 if t2.Before(t1){return errors.New("finished_at earlier than started_at")}
 seen:=map[string]string{}
 for _,c:=range array(d["components"]){m:=object(c);if err:=required(m,"component","status");err!=nil{return err};key,st:=stringOf(m["component"]),stringOf(m["status"]);if key==""||!enum(st,"ok","partial","failed","skipped"){return errors.New("invalid component status")};if _,ok:=seen[key];ok{return errors.New("duplicate component")};seen[key]=st}
 eng:=map[string]bool{};degraded:=false;useful:=false;success:=false
 for _,s:=range seen{if s=="ok"{success=true};if s=="ok"||s=="partial"{useful=true};if s=="failed"||s=="partial"{degraded=true}}
 for _,e:=range array(d["engines"]){m:=object(e);if err:=required(m,"name","status");err!=nil{return err};name,st:=stringOf(m["name"]),stringOf(m["status"]);if !enum(st,"ok","partial","failed","skipped"){return errors.New("invalid engine status")};if eng[name]{return errors.New("duplicate engine")};eng[name]=true;p:=strings.SplitN(name,"/",2)[0];parent,ok:=seen[p];if !ok{return errors.New("engine has no matching component")};if parent=="skipped"&&st!="skipped"{return errors.New("skipped component attempted engine")};if (st=="failed"||st=="partial")&&(parent=="ok"||parent=="skipped"){return errors.New("degraded engine contradicts parent")};if st=="failed"||st=="partial"{degraded=true}}
 for _,e:=range array(d["errors"]){m:=object(e);if err:=required(m,"component","code");err!=nil{return err};st,ok:=seen[stringOf(m["component"])];if !ok{return errors.New("error references missing component")};if st=="ok"||st=="skipped"{return errors.New("error contradicts component")};degraded=true}
 cloudStatus,cloudExists:=seen["cloud"];clouds:=array(d["cloud_checks"])
 if len(clouds)>0&&!cloudExists{return errors.New("cloud_checks require cloud component")}
 if len(clouds)>0&&cloudStatus=="skipped"{return errors.New("skipped cloud component with checks")}
 for _,cc:=range clouds{m:=object(cc);if err:=required(m,"probes","conclusion");err!=nil{return err}
  for _,p:=range array(m["probes"]){probe:=object(p);status:=stringOf(probe["status"]);if !enum(status,"passed","failed","indeterminate","not_tested","unsupported"){return errors.New("invalid cloud probe status")}
   if status=="failed"&&cloudStatus!="partial"&&cloudStatus!="failed"{return errors.New("failed cloud probe contradicts cloud component")}
   if stringOf(probe["http_status"])=="401"&&stringOf(m["conclusion"])=="connection_failed"{return errors.New("HTTP 401 classified incorrectly")}
   if n,ok:=probe["http_status"].(json.Number);ok&&string(n)=="401"&&stringOf(m["conclusion"])=="connection_failed"{return errors.New("HTTP 401 classified incorrectly")}
  }
 }
 overall:=stringOf(d["overall_status"])
 switch overall {
 case "ok":if !success||degraded{return errors.New("overall_status ok contradicts component state")}
 case "partial":if !useful||!degraded{return errors.New("overall_status partial requires usable and degraded")}
 case "failed":if useful||!degraded{return errors.New("overall_status failed contradicts results")};for _,e:=range array(d["engines"]){if enum(stringOf(object(e)["status"]),"ok","partial"){return errors.New("failed overall with usable engine")}}
 default:return errors.New("invalid overall status")
 }
 gpus:=map[string]bool{}
 for _,g:=range array(object(d["hardware"])["gpus"]){idx:=fmt.Sprint(object(g)["index"]);if gpus[idx]{return errors.New("duplicate GPU index")};gpus[idx]=true}
 for _,r:=range array(d["runtime_checks"]){s:=stringOf(object(r)["launch_status"]);if !enum(s,"passed","failed","not_tested","unsupported"){return errors.New("invalid runtime launch_status")}}
 for _,as:=range array(d["model_assessments"]){
  m:=object(as);execution:=object(m["execution"]);if execution==nil{return errors.New("missing assessment execution")}
  if se:=m["speed_estimate"];se!=nil {e:=object(se);if !reflect.DeepEqual(e["execution"],m["execution"]){return errors.New("speed estimate execution differs")};if e["backend_id"]!=execution["backend_id"]{return errors.New("speed estimate backend differs")};if stringOf(object(e["evidence"])["kind"])!="estimated"{return errors.New("speed estimate evidence must be estimated")}}
  for _,measurement:=range array(m["measurements"]){e:=object(measurement)
   if !reflect.DeepEqual(e["execution"],m["execution"]){return errors.New("measurement execution differs")}
   units:=map[string]string{"generation_tps":"tokens/s","prompt_tps":"tokens/s","ttft_ms":"ms","total_latency_ms":"ms","peak_working_set_bytes":"bytes"}
   if units[stringOf(e["kind"])]==""||units[stringOf(e["kind"])]!=stringOf(e["unit"]){return errors.New("measurement kind/unit mismatch")}
   if stringOf(object(e["evidence"])["kind"])!="measured"{return errors.New("measurement evidence must be measured")}
  }
 }
 return nil
}

func validateCatalog(d map[string]any)error{
 if err:=required(d,"catalog_version","generated_at","source_snapshots","models","llmfit_overrides");err!=nil{return err}
 for _,k:=range []string{"source_snapshots","models","llmfit_overrides"}{if err:=requireArray(d,k);err!=nil{return err}}
 sources:=map[string]bool{}
 for _,v:=range array(d["source_snapshots"]){id:=stringOf(object(v)["source_id"]);if id==""||sources[id]{return errors.New("duplicate/invalid source_id")};sources[id]=true}
 models:=map[string]bool{};artifacts:=map[string]bool{}
 for _,v:=range array(d["models"]){
  m:=object(v);id:=stringOf(m["model_id"]);if id==""||models[id]{return errors.New("duplicate model_id")};models[id]=true
  expected:=map[string]bool{};for _,key:=range []string{"architecture","parameters","context_tokens"}{if m[key]!=nil{expected[key]=true}}
  if len(array(m["modalities"]))>0{expected["modalities"]=true}
  for i,v:=range array(m["artifacts"]){
   a:=object(v);aid:=stringOf(a["artifact_id"]);if aid==""||artifacts[aid]{return errors.New("duplicate artifact_id")};artifacts[aid]=true
   for _,k:=range []string{"format","quantization","size_bytes","revision","sha256","source_uri"}{if val,ok:=a[k];ok&&val!=nil{expected[fmt.Sprintf("artifacts[%d].%s",i,k)]=true}}
  }
  prov:=object(m["field_provenance"]);if len(prov)!=len(expected){return errors.New("field_provenance mismatch")}
  for k:=range expected{if len(array(prov[k]))==0{return errors.New("field_provenance missing")}}
  for k,records:=range prov{if !expected[k]{return errors.New("field_provenance extra path")};for _,r:=range array(records){if !sources[stringOf(object(r)["source_id"])]{return errors.New("unknown provenance source")}}}
  for _,a:=range array(m["aliases"]){if !sources[stringOf(object(a)["source_id"])]{return errors.New("alias unknown provenance source")}}
 }
 seen:=map[string]bool{}
 for _,v:=range array(d["llmfit_overrides"]){o:=object(v);if !models[stringOf(o["target_model_id"])]||!sources[stringOf(o["source_id"])]{return errors.New("unknown override model/provenance")}
  if stringOf(o["status"])!="not_applied"{return errors.New("override not CLI verified")}
  key:=stringOf(o["target_model_id"])+"/"+stringOf(o["target_slug"])+"/"+stringOf(o["field"])
  if seen[key]{return errors.New("duplicate llmfit override")};seen[key]=true
 }
 return nil
}
var pointerPattern=regexp.MustCompile("^/data/catalog/manifests/[0-9]{4}\.[0-9]{2}\.[0-9]{2}\.[1-9][0-9]*/manifest\.json$")
func validatePointer(d map[string]any)error{
 if err:=required(d,"manifest_path");err!=nil{return err}
 if !pointerPattern.MatchString(stringOf(d["manifest_path"])){return errors.New("unsafe manifest path")}
 return nil
}
func validateManifest(d map[string]any)error{
 if err:=required(d,"catalog_version","catalog_schema_version","published_at","compatible_diagnostic_schema_versions","min_agent_version","signing_key_id","artifacts");err!=nil{return err}
 if d["catalog_schema_version"]!=Version||!contains(array(d["compatible_diagnostic_schema_versions"]),Version){return errors.New("incompatible manifest version")}
 artifacts:=array(d["artifacts"]);if len(artifacts)==0{return errors.New("empty manifest artifacts")}
 for _,a:=range artifacts{m:=object(a);if m["path"]!="/data/catalog/versions/"+stringOf(d["catalog_version"])+"/catalog.json"{return errors.New("manifest version/path mismatch")}
  h:=stringOf(m["sha256"]);if len(h)!=64{ return errors.New("invalid manifest digest")};if _,err:=hex.DecodeString(h);err!=nil{return errors.New("invalid manifest digest")}
  n,ok:=m["bytes"].(json.Number);if !ok{return errors.New("invalid manifest bytes")};size,err:=n.Int64();if err!=nil||size<1||size>CatalogLimit{return errors.New("invalid manifest size")}
 }
 return nil
}
// SnapshotIntegrity deliberately requires a trusted signature result from a future verifier.
// It does NOT verify Ed25519. Passing signatureVerified=true in a test is not proof of trust.
func SnapshotIntegrity(manifest map[string]any,raw []byte,signatureVerified bool)error{
 if !signatureVerified{return errors.New("unsigned/unverified manifest")}
 b,err:=json.Marshal(manifest);if err!=nil{return err}
 if _,err:=Validate("manifest",b);err!=nil{return err}
 arts:=array(manifest["artifacts"]);if len(arts)!=1{return errors.New("unexpected catalog artifact count")}
 a:=object(arts[0]);n,_:=a["bytes"].(json.Number);expected,_:=n.Int64()
 if int64(len(raw))!=expected{return errors.New("catalog length mismatch")}
 if hex.EncodeToString(sha256sum(raw))!=stringOf(a["sha256"]){return errors.New("catalog digest mismatch")}
 _,err=Validate("catalog",raw);return err
}

func ComparisonStatus(estimated,measured map[string]any)string{
 if estimated["speed_estimate"]==nil||len(array(measured["measurements"]))==0{return "not_comparable"}
 for _,key:=range []string{"model_id","artifact_id"}{if estimated[key]==nil||measured[key]==nil||estimated[key]!=measured[key]{return "not_comparable"}}
 a,b:=object(estimated["execution"]),object(measured["execution"])
 if a==nil||b==nil||a["device_path"]=="unknown"{return "not_comparable"}
 for _,k:=range []string{"device_path","backend_id","runtime_id","runtime_version","context_tokens","threads","gpu_layers"}{if a[k]==nil||b[k]==nil||!reflect.DeepEqual(a[k],b[k]){return "not_comparable"}}
 for _,v:=range array(measured["measurements"]){m:=object(v);if m["kind"]=="generation_tps"&&reflect.DeepEqual(m["execution"],b){return "comparable_configuration"}}
 return "not_comparable"
}
// ExpectedFiles returns the exact set of pinned paths for an external diff/audit.
func ExpectedFiles(root string)([]string,error){
 b,err:=os.ReadFile(filepath.Join(root,"contracts","contract.lock.json"));if err!=nil{return nil,err}
 var l PinLock;if err=json.Unmarshal(b,&l);err!=nil{return nil,err};paths:=[]string{}
 for _,f:=range l.Files{paths=append(paths,f.Path)};sort.Strings(paths);return paths,nil
}
