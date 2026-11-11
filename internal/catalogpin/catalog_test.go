package catalogpin

import (
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "github.com/Herurg123/neurometr-agent/internal/contract"
)

func root(t *testing.T)string{
 t.Helper()
 p,err:=filepath.Abs(filepath.Join("..",".."))
 if err!=nil{t.Fatal(err)}
 return p
}
func validator(t *testing.T)*contract.FullValidator{
 t.Helper()
 v,err:=contract.NewFullValidator(root(t))
 if err!=nil{t.Fatal(err)}
 return v
}
func TestApprovedBundledCatalogExactBytesAndSchema(t *testing.T){
 v:=validator(t)
 ref,err:=Verify(root(t),v)
 if err!=nil{t.Fatalf("approved bundled catalog rejected: %v",err)}
 if ref.Version!=CatalogVersion||ref.SHA256!=CatalogSHA256||ref.Origin!="bundled"||ref.Compatibility!="passed"{
  t.Fatalf("bad verified reference: %+v",ref)
 }
 raw,err:=os.ReadFile(filepath.Join(root(t),"bundled","catalog.json"));if err!=nil{t.Fatal(err)}
 if len(raw)!=CatalogSize{t.Fatalf("catalog bytes %d, want %d",len(raw),CatalogSize)}
 h:=sha256.Sum256(raw)
 if hex.EncodeToString(h[:])!=CatalogSHA256{t.Fatal("pinned bytes SHA256 mismatch")}
 doc,err:=v.ValidateBytes("catalog",raw)
 if err!=nil{t.Fatal(err)}
 if doc["catalog_version"]!=CatalogVersion||doc["schema_version"]!=SchemaVersion{t.Fatal("approved catalog version mismatch")}
 models,ok:=doc["models"].([]any);if !ok||len(models)!=2{t.Fatal("expected two approved Qwen models")}
 artifactCount:=0
 for _,entry:=range models{
  model:=entry.(map[string]any)
  artifactCount+=len(model["artifacts"].([]any))
 }
 if artifactCount!=2{t.Fatalf("expected two approved GGUF artifacts, got %d",artifactCount)}
 t.Log("A3_3_BUNDLED_PIN=PASS BYTES=6589 MODELS=2 ARTIFACTS=2 FULL_VALIDATOR=PASS")
}
func copyBundle(t *testing.T)(string,[]byte,[]byte){
 t.Helper()
 base:=root(t)
 b,err:=os.ReadFile(filepath.Join(base,"bundled","catalog.json"));if err!=nil{t.Fatal(err)}
 lock,err:=os.ReadFile(filepath.Join(base,"bundled","catalog.lock.json"));if err!=nil{t.Fatal(err)}
 dst:=t.TempDir()
 if err=os.MkdirAll(filepath.Join(dst,"bundled"),0700);err!=nil{t.Fatal(err)}
 if err=os.WriteFile(filepath.Join(dst,"bundled","catalog.json"),b,0600);err!=nil{t.Fatal(err)}
 if err=os.WriteFile(filepath.Join(dst,"bundled","catalog.lock.json"),lock,0600);err!=nil{t.Fatal(err)}
 return dst,b,lock
}
func TestTamperMissingLockAndVersionFailClosed(t *testing.T){
 v:=validator(t)
 cases:=[]struct{name string;change func(t *testing.T,dir string,b,lock []byte)}{
  {"missing_catalog",func(t *testing.T,d string,b,l []byte){if err:=os.Remove(filepath.Join(d,"bundled","catalog.json"));err!=nil{t.Fatal(err)}}},
  {"missing_lock",func(t *testing.T,d string,b,l []byte){if err:=os.Remove(filepath.Join(d,"bundled","catalog.lock.json"));err!=nil{t.Fatal(err)}}},
  {"shorter_length",func(t *testing.T,d string,b,l []byte){write(t,d,"catalog.json",b[:len(b)-1])}},
  {"longer_length",func(t *testing.T,d string,b,l []byte){write(t,d,"catalog.json",append(b,' '))}},
  {"corrupted_same_length",func(t *testing.T,d string,b,l []byte){x:=bytes.Clone(b);x[len(x)/2]^=1;write(t,d,"catalog.json",x)}},
  {"wrong_catalog_version",func(t *testing.T,d string,b,l []byte){
   x:=bytes.Replace(b,[]byte(CatalogVersion),[]byte("2026.11.11.3"),1)
   if bytes.Equal(x,b)||len(x)!=len(b){t.Fatal("failed to create equal-length version mutation")}
   write(t,d,"catalog.json",x)
  }},
  {"wrong_locked_sha256",func(t *testing.T,d string,b,l []byte){
   x:=bytes.Replace(l,[]byte(CatalogSHA256),[]byte(strings.Repeat("0",64)),1)
   if bytes.Equal(x,l){t.Fatal("lock digest not found")}
   write(t,d,"catalog.lock.json",x)
  }},
  {"wrong_locked_length",func(t *testing.T,d string,b,l []byte){
   x:=bytes.Replace(l,[]byte("6589"),[]byte("6588"),1)
   if bytes.Equal(x,l){t.Fatal("lock size not found")}
   write(t,d,"catalog.lock.json",x)
  }},
  {"unapproved_source_commit",func(t *testing.T,d string,b,l []byte){
   x:=bytes.Replace(l,[]byte(SourceCommit),[]byte(strings.Repeat("0",40)),1)
   if bytes.Equal(x,l){t.Fatal("lock source commit not found")}
   write(t,d,"catalog.lock.json",x)
  }},
  {"unapproved_locked_version",func(t *testing.T,d string,b,l []byte){
   x:=bytes.Replace(l,[]byte(CatalogVersion),[]byte("2026.11.11.3"),1)
   if bytes.Equal(x,l){t.Fatal("lock catalog version not found")}
   write(t,d,"catalog.lock.json",x)
  }},
  {"duplicate_lock_keys",func(t *testing.T,d string,b,l []byte){
   x:=bytes.Replace(l,[]byte("{"),[]byte("{\"source_repo\":\"untrusted\","),1)
   write(t,d,"catalog.lock.json",x)
  }},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   dir,b,l:=copyBundle(t)
   tc.change(t,dir,b,l)
   ref,err:=Verify(dir,v)
   if !errors.Is(err,ErrUntrusted)||ref!=(Reference{}){
    t.Fatalf("tampered bundled catalog trusted: ref=%+v err=%v",ref,err)
   }
   t.Logf("A3_3_CATALOG_NEGATIVE=%s REJECTED=PASS",tc.name)
  })
 }
}
func write(t *testing.T,dir,file string,b []byte){
 t.Helper()
 if err:=os.WriteFile(filepath.Join(dir,"bundled",file),b,0600);err!=nil{t.Fatal(err)}
}
