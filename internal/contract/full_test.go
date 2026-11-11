package contract

import (
 "bytes"
 "encoding/json"
 "os"
 "strings"
 "testing"
)

func fullValidator(t *testing.T) *FullValidator {
 t.Helper()
 v, err := NewFullValidator(root())
 if err != nil { t.Fatal(err) }
 return v
}

func TestFullValidatorCanonicalF01F08(t *testing.T) {
 v := fullValidator(t)
 rows := []struct { id, kind, file string; accepted bool }{
  {"F01","diagnostic","F01_diagnostic_windows_ok.json",true},
  {"F02","diagnostic","F02_diagnostic_llmfit_timeout.json",true},
  {"F03","diagnostic","F03_diagnostic_offline_site.json",true},
  {"F04","diagnostic","F04_diagnostic_unknown_igpu.json",true},
  {"F05","diagnostic","F05_diagnostic_cpu_vs_sycl.json",true},
  {"F06","diagnostic","F06_diagnostic_unsupported_schema.json",false},
  {"F07","catalog","F07_catalog_invalid_checksum.json",true},
  {"F08","catalog","F08_catalog_incompatible_override.json",false},
  {"F07-manifest","manifest","F07_catalog_invalid_checksum.manifest.json",true},
  {"pointer","pointer","latest_valid.json",true},
  {"catalog","catalog","catalog_valid.json",true},
 }
 for _, row := range rows {
  t.Run(row.id, func(t *testing.T) {
   _, err := v.ValidateBytes(row.kind,readFixture(t,row.file))
   if (err == nil) != row.accepted {
    t.Fatalf("accept=%v, expected=%v: %v",err == nil,row.accepted,err)
   }
  })
 }
 t.Run("F07_checksum_and_unverified_signature",func(t *testing.T){
  catalog:=readFixture(t,"F07_catalog_invalid_checksum.json")
  manifestBytes:=readFixture(t,"F07_catalog_invalid_checksum.manifest.json")
  manifest,err:=v.ValidateBytes("manifest",manifestBytes);if err!=nil{t.Fatal(err)}
  if _,err=v.ValidateBytes("catalog",catalog);err!=nil{t.Fatal(err)}
  if err=SnapshotIntegrity(manifest,catalog,true);err==nil{t.Fatal("invalid SHA-256/size accepted")}
  if err=SnapshotIntegrity(manifest,catalog,false);err==nil{t.Fatal("unverified signature accepted")}
 })
 t.Run("F05_CPU_SYCL_not_comparable",func(t *testing.T){
  d:=decodeFixture(t,"F05_diagnostic_cpu_vs_sycl.json")
  a:=array(d["model_assessments"])
  if len(a)!=2||ComparisonStatus(object(a[1]),object(a[0]))!="not_comparable"{
   t.Fatal("CPU-only measurement and GPU/SYCL estimate misclassified")
  }
 })
}

func TestFullValidatorNestedSchemaFormatAndSemanticMutations(t *testing.T){
 v:=fullValidator(t)
 diag:=decodeFixture(t,"F01_diagnostic_windows_ok.json")
 catalog:=decodeFixture(t,"catalog_valid.json")
 manifest:=decodeFixture(t,"F07_catalog_invalid_checksum.manifest.json")
 pointer:=decodeFixture(t,"latest_valid.json")
 type mutation struct{ name,kind string; source map[string]any; change func(map[string]any) }
 rows:=[]mutation{
  {"missing_required_run_id","diagnostic",diag,func(d map[string]any){delete(d,"run_id")}},
  {"missing_nested_os","diagnostic",diag,func(d map[string]any){delete(object(d["hardware"]),"os")}},
  {"run_id_uuid_format","diagnostic",diag,func(d map[string]any){d["run_id"]="not-a-uuid"}},
  {"started_at_format","diagnostic",diag,func(d map[string]any){d["started_at"]="not-a-date"}},
  {"invalid_nested_cpu_type","diagnostic",diag,func(d map[string]any){object(object(d["hardware"])["cpu"])["physical_cores"]="four"}},
  {"unknown_component_status","diagnostic",diag,func(d map[string]any){object(array(d["components"])[0])["status"]="unknown"}},
  {"array_type_engines","diagnostic",diag,func(d map[string]any){d["engines"]=map[string]any{}}},
  {"duplicate_engine_semantics","diagnostic",diag,func(d map[string]any){e:=array(d["engines"]);d["engines"]=append(e,e[0])}},
  {"date_order_semantics","diagnostic",diag,func(d map[string]any){d["finished_at"]="2020-01-01T00:00:00Z"}},
  {"missing_cloud_semantics","diagnostic",diag,func(d map[string]any){d["components"]=[]any{map[string]any{"component":"hardware","status":"ok"},map[string]any{"component":"llmfit","status":"ok"}}}},
  {"unexpected_root_property","diagnostic",diag,func(d map[string]any){d["surprise"]="injected"}},
  {"unexpected_nested_property","diagnostic",diag,func(d map[string]any){object(object(d["hardware"])["cpu"])["surprise"]=42}},
  {"bad_evidence_kind","diagnostic",diag,func(d map[string]any){object(object(object(d["hardware"])["cpu"])["evidence"])["kind"]="working"}},
  {"invalid_execution_consistency","diagnostic",diag,func(d map[string]any){a:=object(array(d["model_assessments"])[0]);object(object(a["speed_estimate"])["execution"])["device_path"]="gpu_offload"}},
  {"unsafe_nested_integer","diagnostic",diag,func(d map[string]any){object(object(d["hardware"])["ram"])["physical_total_bytes"]=json.Number("9007199254740992")}},
  {"missing_catalog_models","catalog",catalog,func(d map[string]any){delete(d,"models")}},
  {"invalid_catalog_numeric_type","catalog",catalog,func(d map[string]any){object(array(d["models"])[0])["parameters"]="billion"}},
  {"missing_field_provenance","catalog",catalog,func(d map[string]any){object(array(d["models"])[0])["field_provenance"]=map[string]any{}}},
  {"catalog_extra_model_property","catalog",catalog,func(d map[string]any){object(array(d["models"])[0])["unapproved"]="x"}},
  {"catalog_duplicate_source","catalog",catalog,func(d map[string]any){s:=array(d["source_snapshots"]);d["source_snapshots"]=append(s,s[0])}},
  {"catalog_unknown_provenance_source","catalog",catalog,func(d map[string]any){m:=object(array(d["models"])[0]);p:=object(m["field_provenance"]);object(array(p["parameters"])[0])["source_id"]="missing"}},
  {"manifest_unsupported_version","manifest",manifest,func(d map[string]any){d["catalog_schema_version"]="99.0.0"}},
  {"manifest_invalid_artifact_hash","manifest",manifest,func(d map[string]any){object(array(d["artifacts"])[0])["sha256"]="invalid"}},
  {"manifest_wrong_artifact_path","manifest",manifest,func(d map[string]any){object(array(d["artifacts"])[0])["path"]="/data/catalog/versions/other/catalog.json"}},
  {"manifest_missing_required","manifest",manifest,func(d map[string]any){delete(d,"signing_key_id")}},
  {"pointer_external_url","pointer",pointer,func(d map[string]any){d["manifest_path"]="https://attacker.invalid/m.json"}},
  {"pointer_path_traversal","pointer",pointer,func(d map[string]any){d["manifest_path"]="../catalog.json"}},
  {"pointer_extra_property","pointer",pointer,func(d map[string]any){d["extra"]="unapproved"}},
 }
 for _,row:=range rows{
  t.Run(row.name,func(t *testing.T){
   raw,err:=json.Marshal(row.source);if err!=nil{t.Fatal(err)}
   // Never mutate the pinned fixtures or the shared base object.
   var cloned map[string]any
   dec:=json.NewDecoder(bytes.NewReader(raw));dec.UseNumber()
   if err=dec.Decode(&cloned);err!=nil{t.Fatal(err)}
   row.change(cloned)
   mutated,err:=json.Marshal(cloned);if err!=nil{t.Fatal(err)}
   if _,err=v.ValidateBytes(row.kind,mutated);err==nil{t.Fatalf("%s unexpectedly accepted",row.name)}
  })
 }
}

func TestFullValidatorStrictInputBudgets(t *testing.T){
 v:=fullValidator(t)
 t.Run("duplicate_keys",func(t *testing.T){if _,err:=v.ValidateBytes("diagnostic",[]byte(`{"a":1,"a":2}`));err==nil{t.Fatal("duplicate keys accepted")}})
 t.Run("invalid_utf8",func(t *testing.T){if _,err:=v.ValidateBytes("diagnostic",[]byte{0x7b,0x22,0xff,0x22,0x3a,0x31,0x7d});err==nil{t.Fatal("invalid utf8 accepted")}})
 t.Run("size_2MiB",func(t *testing.T){if _,err:=v.ValidateBytes("diagnostic",bytes.Repeat([]byte(" "),DiagnosticLimit+1));err==nil{t.Fatal("oversized diagnostic accepted")}})
 t.Run("depth_64",func(t *testing.T){payload:=strings.Repeat("[",65)+"0"+strings.Repeat("]",65);if _,err:=v.ValidateBytes("diagnostic",[]byte(payload));err==nil{t.Fatal("oversized depth accepted")}})
 t.Run("nodes_100000",func(t *testing.T){payload:="["+strings.Repeat("0,",100000)+"0]";if _,err:=v.ValidateBytes("diagnostic",[]byte(payload));err==nil{t.Fatal("oversized node count accepted")}})
 t.Run("unsafe_number",func(t *testing.T){if _,err:=v.ValidateBytes("diagnostic",[]byte(`{"n":9007199254740992}`));err==nil{t.Fatal("unsafe numeric accepted")}})
 t.Run("unknown_kind",func(t *testing.T){if _,err:=v.ValidateBytes("unknown",[]byte("{}"));err==nil{t.Fatal("unknown kind accepted")}})
}

func TestFullValidatorPortableNumericProfile(t *testing.T){
 v:=fullValidator(t)
 b:=readFixture(t,"numeric-vectors.json")
 var corpus struct{Vectors []struct{Input string `json:"input"`;Accept bool `json:"accept"`} `json:"vectors"`}
 if err:=json.Unmarshal(b,&corpus);err!=nil{t.Fatal(err)}
 if len(corpus.Vectors)!=23{t.Fatalf("got %d vectors",len(corpus.Vectors))}
 for _,row:=range corpus.Vectors{
  t.Run(row.Input,func(t *testing.T){
   // The numeric vectors specify parser acceptance, not diagnostic schema acceptance.
   _,err:=DecodeBounded([]byte(`{"value":`+row.Input+"}"),DiagnosticLimit)
   if (err==nil)!=row.Accept{t.Fatalf("input=%s accepted=%v want=%v err=%v",row.Input,err==nil,row.Accept,err)}
  })
 }
 // Constructor verified the pinned source; the additional mutators must not alter it.
 if err:=VerifyPin(root());err!=nil{t.Fatal(err)}
 // An untrusted schema is never fetched from a URL in imported JSON.
 if err:=localRefsOnly(map[string]any{"$ref":"https://attacker.invalid/schema.json"});err==nil{
  t.Fatal("remote JSON Schema reference accepted")
 }
 // Test the original approved diagnostic remains valid after all scenarios.
 raw,err:=os.ReadFile(filepathJoinFixture("F01_diagnostic_windows_ok.json"))
 if err!=nil{t.Fatal(err)}
 if _,err=v.ValidateBytes("diagnostic",raw);err!=nil{t.Fatal(err)}
}
func filepathJoinFixture(name string)string{return root()+"/contracts/fixtures/"+name}
