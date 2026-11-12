package history

import (
 "bytes"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "testing"

 "github.com/Herurg123/neurometr-agent/internal/contract"
)

func testStore(t *testing.T)(*Store,[]byte,string){
 t.Helper()
 root,err:=filepath.Abs(filepath.Join("..",".."));if err!=nil{t.Fatal(err)}
 v,err:=contract.NewFullValidator(root);if err!=nil{t.Fatal(err)}
 dir:=filepath.Join(t.TempDir(),"private-history")
 s,err:=New(dir,v);if err!=nil{t.Fatalf("private history setup: %v",err)}
 raw,err:=os.ReadFile(filepath.Join(root,"contracts","fixtures","F01_diagnostic_windows_ok.json"));if err!=nil{t.Fatal(err)}
 doc,err:=v.ValidateBytes("diagnostic",raw);if err!=nil{t.Fatal(err)}
 return s,raw,doc["run_id"].(string)
}
func TestPrivateHistoryRestartNoOverwrite(t *testing.T){
 s,raw,id:=testStore(t)
 entry,err:=s.Save(raw)
 if err!=nil{t.Fatal(err)}
 if entry.ID!=id{t.Fatal("history id not from validated report")}
 if _,err=s.Save(raw);!errors.Is(err,ErrExists){t.Fatalf("overwrite was not blocked: %v",err)}
 paths,err:=os.ReadDir(s.dir);if err!=nil{t.Fatal(err)}
 if len(paths)!=1||paths[0].Name()!=id+".json"{t.Fatal("partial temp file left or invalid safe name")}
 reopened,err:=New(s.dir,s.validator);if err!=nil{t.Fatal(err)}
 list,err:=reopened.List();if err!=nil{t.Fatal(err)}
 if len(list.Entries)!=1||list.Entries[0].ID!=id{t.Fatal("restart failed to find valid historical report")}
 got,doc,err:=reopened.Load(id)
 if err!=nil||!bytes.Equal(got,raw)||doc["run_id"]!=id{t.Fatal("stored report changed or not revalidated")}
 t.Log("A3_4_HISTORY_RESTART=PASS NO_OVERWRITE=PASS FULL_VALIDATOR=PASS")
}
func TestCorruptOversizedUnsupportedAndTraversalIsolation(t *testing.T){
 s,raw,id:=testStore(t)
 if _,err:=s.Save(raw);err!=nil{t.Fatal(err)}
 invalidIDs:=[]string{"../etc/passwd","..","C:\\Users\\Admin\\secret.json","",id+"/../x",strings.ToUpper(id),"%2e%2e",id+".json"}
 for _,name:=range invalidIDs{
  if _,_,err:=s.Load(name);!errors.Is(err,ErrInvalidID){t.Fatalf("unsafe ID accepted: %q",name)}
 }
 badID:="42a94700-ea8f-40b5-8dab-bb14d43005d7"
 if err:=os.WriteFile(filepath.Join(s.dir,badID+".json"),[]byte("{"),0600);err!=nil{t.Fatal(err)}
 largeID:="42a94700-ea8f-40b5-8dab-bb14d43005d8"
 if err:=os.WriteFile(filepath.Join(s.dir,largeID+".json"),bytes.Repeat([]byte("a"),contract.DiagnosticLimit+1),0600);err!=nil{t.Fatal(err)}
 futureID:="42a94700-ea8f-40b5-8dab-bb14d43005d9"
 future:=bytes.Replace(raw,[]byte(`"schema_version": "0.1.0"`),[]byte(`"schema_version": "99.0.0"`),1)
 if bytes.Equal(future,raw){t.Fatal("version mutation failed")}
 if err:=os.WriteFile(filepath.Join(s.dir,futureID+".json"),future,0600);err!=nil{t.Fatal(err)}
 otherID:="42a94700-ea8f-40b5-8dab-bb14d43005e0"
 if err:=os.WriteFile(filepath.Join(s.dir,otherID+".json"),raw,0600);err!=nil{t.Fatal(err)}
 for _,bad:=range []string{badID,largeID,futureID,otherID}{
  if _,_,err:=s.Load(bad);!errors.Is(err,ErrCorrupt){t.Fatalf("bad/oversized/version-mismatch report opened: %s: %v",bad,err)}
 }
 listing,err:=s.List()
 if err!=nil{t.Fatal(err)}
 if len(listing.Entries)!=1||listing.Entries[0].ID!=id||listing.InvalidCount<4{
  t.Fatalf("corrupted report prevented valid history: %+v",listing)
 }
 if _,_,err=s.Load(id);err!=nil{t.Fatal("valid report lost after corrupt neighbors")}
 t.Log("A3_4_HISTORY_CORRUPT_ISOLATED=PASS UNSUPPORTED_VERSION=REJECTED OVERSIZE=REJECTED TRAVERSAL=REJECTED")
}
func TestAtomicHistoryConcurrentWriters(t *testing.T){
 s,raw,_:=testStore(t)
 var wg sync.WaitGroup
 outcomes:=make(chan error,12)
 for i:=0;i<12;i++{wg.Add(1);go func(){defer wg.Done();_,err:=s.Save(raw);outcomes<-err}()}
 wg.Wait();close(outcomes)
 countOk,countExists:=0,0
 for err:=range outcomes{
  switch {
  case err==nil:countOk++
  case errors.Is(err,ErrExists):countExists++
  default:t.Fatalf("unexpected concurrent writer failure: %v",err)
  }
 }
 if countOk!=1||countExists!=11{t.Fatalf("same run overwritten: ok=%d exists=%d",countOk,countExists)}
 files,err:=os.ReadDir(s.dir);if err!=nil{t.Fatal(err)}
 if len(files)!=1{t.Fatal("atomic save left temporary/duplicate files")}
}
func TestStoreRejectsSymlinkedReport(t *testing.T){
 s,raw,id:=testStore(t)
 outside:=filepath.Join(t.TempDir(),"outside.json")
 if err:=os.WriteFile(outside,raw,0600);err!=nil{t.Fatal(err)}
 if err:=os.Symlink(outside,filepath.Join(s.dir,id+".json"));err!=nil{
  t.Skip("symlink creation disallowed for unprivileged Windows runner")
 }
 if _,_,err:=s.Load(id);!errors.Is(err,ErrCorrupt){t.Fatalf("symlinked report followed: %v",err)}
}
