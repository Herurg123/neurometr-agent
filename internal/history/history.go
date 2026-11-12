// Package history stores canonical diagnostic reports privately without a
// network backend, automatic uploads, rotation or deletion.
package history

import (
 "bytes"
 "crypto/subtle"
 "encoding/json"
 "errors"
 "io"
 "os"
 "path/filepath"
 "regexp"
 "sort"
 "sync"

 "github.com/Herurg123/neurometr-agent/internal/contract"
)

var (
 ErrInvalidID=errors.New("invalid diagnostic identifier")
 ErrExists=errors.New("diagnostic report already exists")
 ErrUnavailable=errors.New("diagnostic history unavailable")
 ErrCorrupt=errors.New("diagnostic is invalid or unsupported")
)
var idPattern=regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type Store struct{
 dir string
 validator *contract.FullValidator
 mu sync.Mutex
}
type Entry struct{
 ID string `json:"id"`
 StartedAt string `json:"started_at"`
 OverallStatus string `json:"overall_status"`
}
type Listing struct{
 Entries []Entry `json:"entries"`
 InvalidCount int `json:"invalid_count"`
 Truncated bool `json:"truncated"`
}
const maxEntries=2000
func DefaultDir()(string,error){
 base,err:=os.UserCacheDir()
 if err!=nil||base==""{return "",ErrUnavailable}
 return filepath.Join(base,"Neurometr","history"),nil
}
// New ensures a per-user directory and verifies Windows directory ACLs at
// runtime. The application CLI never offers a custom history directory.
func New(dir string,v *contract.FullValidator)(*Store,error){
 if v==nil||!filepath.IsAbs(dir){return nil,ErrUnavailable}
 if err:=os.MkdirAll(dir,0700);err!=nil{return nil,ErrUnavailable}
 if err:=ensurePrivateDirectory(dir);err!=nil{return nil,ErrUnavailable}
 return &Store{dir:dir,validator:v},nil
}
func validID(id string)bool{return idPattern.MatchString(id)}
func (s *Store) checked(raw []byte,expectedID string)(map[string]any,error){
 if len(raw)==0||len(raw)>contract.DiagnosticLimit{return nil,ErrCorrupt}
 doc,err:=s.validator.ValidateBytes("diagnostic",raw)
 if err!=nil{return nil,ErrCorrupt}
 id,ok:=doc["run_id"].(string)
 if !ok||!validID(id){return nil,ErrCorrupt}
 if expectedID!=""&&subtle.ConstantTimeCompare([]byte(expectedID),[]byte(id))!=1{
  return nil,ErrCorrupt
 }
 return doc,nil
}
func (s *Store) Save(raw []byte)(Entry,error){
 doc,err:=s.checked(raw,"")
 if err!=nil{return Entry{},err}
 id:=doc["run_id"].(string)
 entry:=Entry{ID:id,StartedAt:doc["started_at"].(string),OverallStatus:doc["overall_status"].(string)}
 s.mu.Lock()
 defer s.mu.Unlock()
 if err=ensurePrivateDirectory(s.dir);err!=nil{return Entry{},ErrUnavailable}
 target:=filepath.Join(s.dir,id+".json")
 if _,err=os.Lstat(target);err==nil{return Entry{},ErrExists}else if !errors.Is(err,os.ErrNotExist){return Entry{},ErrUnavailable}
 f,err:=os.CreateTemp(s.dir,".pending-*.tmp")
 if err!=nil{return Entry{},ErrUnavailable}
 tmp:=f.Name()
 defer os.Remove(tmp)
 writeErr:=f.Chmod(0600)
 if writeErr==nil{_,writeErr=f.Write(raw)}
 if writeErr==nil{writeErr=f.Sync()}
 closeErr:=f.Close()
 if writeErr!=nil||closeErr!=nil{return Entry{},ErrUnavailable}
 // Atomically move within the same directory, without overwrite.
 if err=moveNoReplace(tmp,target);err!=nil{
  if _,statErr:=os.Lstat(target);statErr==nil{return Entry{},ErrExists}
  return Entry{},ErrUnavailable
 }
 return entry,nil
}
func (s *Store) Load(id string)([]byte,map[string]any,error){
 if !validID(id){return nil,nil,ErrInvalidID}
 if err:=ensurePrivateDirectory(s.dir);err!=nil{return nil,nil,ErrUnavailable}
 path:=filepath.Join(s.dir,id+".json")
 st,err:=os.Lstat(path)
 if err!=nil{return nil,nil,ErrUnavailable}
 if !st.Mode().IsRegular()||st.Size()<=0||st.Size()>contract.DiagnosticLimit{
  return nil,nil,ErrCorrupt
 }
 f,err:=os.Open(path)
 if err!=nil{return nil,nil,ErrUnavailable}
 defer f.Close()
 raw,err:=io.ReadAll(io.LimitReader(f,int64(contract.DiagnosticLimit)+1))
 if err!=nil{return nil,nil,ErrUnavailable}
 doc,err:=s.checked(raw,id)
 if err!=nil{return nil,nil,err}
 return raw,doc,nil
}
func (s *Store) List()(Listing,error){
 result:=Listing{Entries:[]Entry{}}
 if err:=ensurePrivateDirectory(s.dir);err!=nil{return result,ErrUnavailable}
 names,err:=os.ReadDir(s.dir)
 if err!=nil{return result,ErrUnavailable}
 if len(names)>maxEntries {
  result.Truncated=true
 }
 // Sort filenames descending for approximate recent-first ordering, but
 // the actual displayed order uses validated timestamps.
 for _,entry:=range names{
  if len(result.Entries)>=maxEntries{result.Truncated=true;break}
  if entry.IsDir()||filepath.Ext(entry.Name())!=".json"{
   continue
  }
  id:=entry.Name()[:len(entry.Name())-len(".json")]
  if !validID(id){result.InvalidCount++;continue}
  _,doc,e:=s.Load(id)
  if e!=nil{result.InvalidCount++;continue}
  result.Entries=append(result.Entries,Entry{
   ID:id,StartedAt:doc["started_at"].(string),OverallStatus:doc["overall_status"].(string),
  })
 }
 sort.Slice(result.Entries,func(i,j int)bool{
  if result.Entries[i].StartedAt==result.Entries[j].StartedAt{return result.Entries[i].ID>result.Entries[j].ID}
  return result.Entries[i].StartedAt>result.Entries[j].StartedAt
 })
 return result,nil
}
// MarshalFull returns unchanged validated document bytes, not regenerated
// estimates. History never treats catalog references as executable instructions.
func MarshalFull(doc map[string]any)([]byte,error){
 var buf bytes.Buffer
 if err:=json.NewEncoder(&buf).Encode(doc);err!=nil{return nil,ErrCorrupt}
 return buf.Bytes(),nil
}
