package contract

import (
 "bytes"
 "encoding/json"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func root()string{return filepath.Join("..","..")}
func readFixture(t *testing.T,name string)[]byte{
 t.Helper();b,e:=os.ReadFile(filepath.Join(root(),"contracts","fixtures",name));if e!=nil{t.Fatal(e)};return b
}
func decodeFixture(t *testing.T,name string)map[string]any{
 t.Helper();v,e:=DecodeBounded(readFixture(t,name),CatalogLimit);if e!=nil{t.Fatal(e)};return object(v)
}
func checkDiagnostic(t *testing.T,d map[string]any,want bool){
 t.Helper();b,e:=json.Marshal(d);if e!=nil{t.Fatal(e)}
 _,e=Validate("diagnostic",b)
 if (e==nil)!=want{t.Fatalf("valid=%v want=%v: %v",e==nil,want,e)}
}
func checkCatalog(t *testing.T,d map[string]any,want bool){
 t.Helper();b,e:=json.Marshal(d);if e!=nil{t.Fatal(e)}
 _,e=Validate("catalog",b)
 if (e==nil)!=want{t.Fatalf("valid=%v want=%v: %v",e==nil,want,e)}
}
func TestPinExactAndNoMutation(t *testing.T){
 if e:=VerifyPin(root());e!=nil{t.Fatal(e)}
 p,e:=ExpectedFiles(root());if e!=nil{t.Fatal(e)}
 if len(p)!=23{t.Fatalf("expected 23 files, got %d",len(p))}
 // Work in a disposable directory only; never modify the source fixtures.
 tmp:=t.TempDir()
 for _,name:=range append(p,"contracts/contract.lock.json"){
  b,e:=os.ReadFile(filepath.Join(root(),filepath.FromSlash(name)));if e!=nil{t.Fatal(e)}
  dst:=filepath.Join(tmp,filepath.FromSlash(name))
  if e=os.MkdirAll(filepath.Dir(dst),0755);e!=nil{t.Fatal(e)}
  if e=os.WriteFile(dst,b,0644);e!=nil{t.Fatal(e)}
 }
 if e:=VerifyPin(tmp);e!=nil{t.Fatal(e)}
 target:=filepath.Join(tmp,"contracts","fixtures","F01_diagnostic_windows_ok.json")
 b,_:=os.ReadFile(target);b=append(b,byte(' '))
 if e:=os.WriteFile(target,b,0644);e!=nil{t.Fatal(e)}
 if e:=VerifyPin(tmp);e==nil{t.Fatal("silent fixture edit was not detected")}
}
func TestSchemaDraftAndIDs(t *testing.T){
 if e:=VerifySchemaHeaders(root());e!=nil{t.Fatal(e)}
}
func TestPortableNumericVectors(t *testing.T){
 var inventory struct {Profile string `json:"profile"`; Vectors []struct{Input string `json:"input"`;Accept bool `json:"accept"`} `json:"vectors"`}
 if e:=json.Unmarshal(readFixture(t,"numeric-vectors.json"),&inventory);e!=nil{t.Fatal(e)}
 if inventory.Profile!="neurometr-json-numeric-0.1.0"||len(inventory.Vectors)!=23{t.Fatal("numeric corpus not canonical")}
 for _,v:=range inventory.Vectors{
  t.Run(v.Input,func(t *testing.T){
   _,e:=DecodeBounded([]byte(`{"number":`+v.Input+`}`),DiagnosticLimit)
   if (e==nil)!=v.Accept{t.Fatalf("number %q accepted=%v want=%v err=%v",v.Input,e==nil,v.Accept,e)}
  })
 }
}
func TestF01ThroughF08(t *testing.T){
 rows:=[]struct{id,kind,file string;valid bool}{
 {"F01","diagnostic","F01_diagnostic_windows_ok.json",true},
 {"F02","diagnostic","F02_diagnostic_llmfit_timeout.json",true},
 {"F03","diagnostic","F03_diagnostic_offline_site.json",true},
 {"F04","diagnostic","F04_diagnostic_unknown_igpu.json",true},
 {"F05","diagnostic","F05_diagnostic_cpu_vs_sycl.json",true},
 {"F06","diagnostic","F06_diagnostic_unsupported_schema.json",false},
 {"F07","catalog","F07_catalog_invalid_checksum.json",true},
 {"F08","catalog","F08_catalog_incompatible_override.json",false},
 }
 e:=decodeFixture(t,"expected-results.json")
 if len(e)!=8{t.Fatal("missing F01-F08 expected results")}
 for _,row:=range rows{t.Run(row.id,func(t *testing.T){
  _,err:=Validate(row.kind,readFixture(t,row.file))
  want:=object(e[row.id]);if want==nil||want["valid"]!=row.valid{t.Fatal("expected-results inventory mismatch")}
  if (err==nil)!=row.valid{t.Fatalf("valid=%v want=%v: %v",err==nil,row.valid,err)}
 })}
 t.Run("F02_partial_hardware_retained",func(t *testing.T){d:=decodeFixture(t,"F02_diagnostic_llmfit_timeout.json");if d["overall_status"]!="partial"||len(array(d["errors"]))==0||object(object(d["hardware"])["ram"])["physical_total_bytes"]==nil{t.Fatal("partial degradation lost useful hardware")}})
 t.Run("F03_offline_bundled",func(t *testing.T){d:=decodeFixture(t,"F03_diagnostic_offline_site.json");if object(d["catalog_ref"])["origin"]!="bundled"||len(array(d["cloud_checks"]))!=0{t.Fatal("unexpected network state")}})
 t.Run("F04_unknown_not_zero",func(t *testing.T){d:=decodeFixture(t,"F04_diagnostic_unknown_igpu.json");g:=object(array(object(d["hardware"])["gpus"])[0]);if object(g["memory"])["dedicated_total_bytes"]!=nil||stringOf(object(array(object(d["hardware"])["npus"])[0])["detection_status"])!="unknown"{t.Fatal("unknown normalized incorrectly")}})
 t.Run("F05_modes_not_comparable",func(t *testing.T){d:=decodeFixture(t,"F05_diagnostic_cpu_vs_sycl.json");as:=array(d["model_assessments"]);if len(as)!=2||ComparisonStatus(object(as[1]),object(as[0]))!="not_comparable"{t.Fatal("CPU/SYCL comparison incorrectly permitted")}})
 t.Run("F07_untrusted_catalog",func(t *testing.T){
  raw:=readFixture(t,"F07_catalog_invalid_checksum.json")
  manifestRaw:=readFixture(t,"F07_catalog_invalid_checksum.manifest.json")
  manifest,e:=Validate("manifest",manifestRaw);if e!=nil{t.Fatal(e)}
  if e=SnapshotIntegrity(manifest,raw,true);e==nil{t.Fatal("mismatched digest/size accepted")}
  if e=SnapshotIntegrity(manifest,raw,false);e==nil{t.Fatal("unverified signature accepted")}
 })
 t.Run("F08_not_applied_allowed_but_not_proof_of_CLI",func(t *testing.T){
  c:=decodeFixture(t,"F08_catalog_incompatible_override.json")
  object(array(c["llmfit_overrides"])[0])["status"]="not_applied"
  checkCatalog(t,c,true)
 })
}
func TestNegativeSemanticCases(t *testing.T){
 t.Run("missing_cloud_component_with_passed_probes",func(t *testing.T){
  d:=decodeFixture(t,"F01_diagnostic_windows_ok.json")
  comps:=[]any{};for _,c:=range array(d["components"]){if object(c)["component"]!="cloud"{comps=append(comps,c)}};d["components"]=comps
  checkDiagnostic(t,d,false)
 })
 t.Run("missing_cloud_component_with_not_tested_probe",func(t *testing.T){
  d:=decodeFixture(t,"F01_diagnostic_windows_ok.json")
  ps:=array(object(array(d["cloud_checks"])[0])["probes"])
  object(ps[0])["status"]="not_tested"
  comps:=[]any{};for _,c:=range array(d["components"]){if object(c)["component"]!="cloud"{comps=append(comps,c)}};d["components"]=comps
  checkDiagnostic(t,d,false)
 })
 t.Run("unknown_version",func(t *testing.T){d:=decodeFixture(t,"F01_diagnostic_windows_ok.json");d["schema_version"]="9.9.9";checkDiagnostic(t,d,false)})
 t.Run("invalid_component_status",func(t *testing.T){d:=decodeFixture(t,"F01_diagnostic_windows_ok.json");object(array(d["components"])[0])["status"]="unknown";checkDiagnostic(t,d,false)})
 t.Run("partial_requires_degradation",func(t *testing.T){d:=decodeFixture(t,"F01_diagnostic_windows_ok.json");d["overall_status"]="partial";checkDiagnostic(t,d,false)})
 t.Run("failed_cannot_hide_useful_results",func(t *testing.T){d:=decodeFixture(t,"F01_diagnostic_windows_ok.json");d["overall_status"]="failed";checkDiagnostic(t,d,false)})
 t.Run("engine_failed_with_ok_parent",func(t *testing.T){d:=decodeFixture(t,"F01_diagnostic_windows_ok.json");object(array(d["engines"])[0])["status"]="failed";checkDiagnostic(t,d,false)})
 t.Run("error_on_ok_component",func(t *testing.T){d:=decodeFixture(t,"F01_diagnostic_windows_ok.json");d["errors"]=[]any{map[string]any{"component":"llmfit","code":"ERR","message":"error","retryable":false}};checkDiagnostic(t,d,false)})
 t.Run("CPU_nested_execution_differs",func(t *testing.T){d:=decodeFixture(t,"F05_diagnostic_cpu_vs_sycl.json");a:=object(array(d["model_assessments"])[0]);object(object(array(a["measurements"])[0])["execution"])["device_path"]="gpu_offload";checkDiagnostic(t,d,false)})
 t.Run("estimated_evidence_not_measured",func(t *testing.T){d:=decodeFixture(t,"F05_diagnostic_cpu_vs_sycl.json");a:=object(array(d["model_assessments"])[1]);object(object(a["speed_estimate"])["evidence"])["kind"]="measured";checkDiagnostic(t,d,false)})
 t.Run("unknown_memory_nullable",func(t *testing.T){d:=decodeFixture(t,"F04_diagnostic_unknown_igpu.json");checkDiagnostic(t,d,true)})
 t.Run("runtime_not_tested_is_not_failed",func(t *testing.T){d:=decodeFixture(t,"F04_diagnostic_unknown_igpu.json");checkDiagnostic(t,d,true)})
 t.Run("bad_pointer_path",func(t *testing.T){m:=decodeFixture(t,"latest_valid.json");m["manifest_path"]="../catalog.json";b,_:=json.Marshal(m);if _,e:=Validate("pointer",b);e==nil{t.Fatal("path traversal accepted")}})
 t.Run("invalid_manifest_hash",func(t *testing.T){m:=decodeFixture(t,"F07_catalog_invalid_checksum.manifest.json");object(array(m["artifacts"])[0])["sha256"]="invalid";b,_:=json.Marshal(m);if _,e:=Validate("manifest",b);e==nil{t.Fatal("invalid hash accepted")}})
 t.Run("provenance_unknown_source",func(t *testing.T){m:=decodeFixture(t,"catalog_valid.json");model:=object(array(m["models"])[0]);p:=object(model["field_provenance"]);object(array(p["parameters"])[0])["source_id"]="fake";checkCatalog(t,m,false)})
 t.Run("override_duplicate",func(t *testing.T){m:=decodeFixture(t,"F08_catalog_incompatible_override.json");ov:=object(array(m["llmfit_overrides"])[0]);ov["status"]="not_applied";m["llmfit_overrides"]=[]any{ov,ov};checkCatalog(t,m,false)})
 t.Run("duplicate_json_keys",func(t *testing.T){if _,e:=DecodeBounded([]byte(`{"a":{"z":1,"z":2}}`),DiagnosticLimit);e==nil{t.Fatal("duplicate keys accepted")}})
 t.Run("unsafe_numeric_nested",func(t *testing.T){if _,e:=DecodeBounded([]byte(`{"a":[{"x":9007199254740992}]}`),DiagnosticLimit);e==nil{t.Fatal("unsafe integer accepted")}})
 t.Run("nonfinite_numeric",func(t *testing.T){if _,e:=DecodeBounded([]byte(`{"x":NaN}`),DiagnosticLimit);e==nil{t.Fatal("NaN accepted")}})
 t.Run("trailing_JSON",func(t *testing.T){if _,e:=DecodeBounded([]byte(`{"a":1}{"b":2}`),DiagnosticLimit);e==nil{t.Fatal("trailing JSON accepted")}})
 t.Run("size_limit",func(t *testing.T){if _,e:=DecodeBounded(bytes.Repeat([]byte(" "),DiagnosticLimit+1),DiagnosticLimit);e==nil{t.Fatal("size budget not enforced")}})
 t.Run("source_fixtures_unchanged",func(t *testing.T){if e:=VerifyPin(root());e!=nil{t.Fatal(e)}})
}
