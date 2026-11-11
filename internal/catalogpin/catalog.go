// Package catalogpin verifies the exact owner-approved B3.1 offline snapshot.
// Source/lock metadata are immutable assertions, not caller-supplied trust keys.
package catalogpin

import (
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "fmt"
 "os"
 "path/filepath"

 "github.com/Herurg123/neurometr-agent/internal/contract"
)

const (
 SourceRepo = "Herurg123/neurometr-web"
 SourceCommit = "bb828bb09372e16cf63d04371d5f9367eee2526e"
 SourcePath = "catalog/versions/2026.11.11.2/catalog.json"
 CatalogVersion = "2026.11.11.2"
 SchemaVersion = "0.1.0"
 CatalogSize = 6589
 CatalogSHA256 = "1d84ff596c589a4db004f8a3fa17490d5479f57db3d7b0a433a7e448289651d2"
)

var ErrUntrusted = errors.New("bundled catalog identity or compatibility is not verified")

// Reference contains only values derived from the verified immutable snapshot.
// The SHA256 proves identity to this approved offline snapshot, NOT a remote
// publisher signature, Ed25519 authentication, or permission to fetch any URLs.
type Reference struct {
 Version string
 SHA256 string
 Origin string
 Compatibility string
}
func (ref Reference) Fields() map[string]any{
 return map[string]any{
  "version":ref.Version,
  "sha256":ref.SHA256,
  "origin":ref.Origin,
  "compatibility":ref.Compatibility,
 }
}

func regularNoSymlink(path string, maxSize int64) error {
 st,err:=os.Lstat(path)
 if err!=nil{return ErrUntrusted}
 if !st.Mode().IsRegular()||st.Size()<=0||st.Size()>maxSize{return ErrUntrusted}
 return nil
}

// Verify uses only fixed bundled file names. Neither the lock file nor caller
// input controls a filesystem path or a security anchor. Both files are checked
// again every time Generate creates a report, so post-build corruption fails
// closed without manufacturing catalog_ref.
func Verify(root string,v *contract.FullValidator)(Reference,error){
 empty:=Reference{}
 if root==""||v==nil{return empty,ErrUntrusted}
 bundleDir:=filepath.Join(root,"bundled")
 folder,err:=os.Lstat(bundleDir)
 if err!=nil||!folder.IsDir()||folder.Mode()&os.ModeSymlink!=0{
  return empty,ErrUntrusted
 }
 lockPath:=filepath.Join(bundleDir,"catalog.lock.json")
 catalogPath:=filepath.Join(bundleDir,"catalog.json")
 if regularNoSymlink(lockPath,4096)!=nil||regularNoSymlink(catalogPath,CatalogSize)!=nil{
  return empty,ErrUntrusted
 }
 rawLock,err:=os.ReadFile(lockPath)
 if err!=nil{return empty,ErrUntrusted}
 parsed,err:=contract.DecodeBounded(rawLock,4096)
 if err!=nil{return empty,ErrUntrusted}
 lock,ok:=parsed.(map[string]any)
 if !ok||len(lock)!=7{return empty,ErrUntrusted}
 expected:=map[string]any{
  "source_repo":SourceRepo,
  "source_commit":SourceCommit,
  "source_path":SourcePath,
  "catalog_version":CatalogVersion,
  "schema_version":SchemaVersion,
  "sha256":CatalogSHA256,
 }
 for name,want:=range expected{
  if lock[name]!=want{return empty,fmt.Errorf("%w: unexpected lock metadata",ErrUntrusted)}
 }
 // Preserve exact integer syntax for the locked byte length.
 if fmt.Sprint(lock["size_bytes"])!=fmt.Sprint(CatalogSize){
  return empty,fmt.Errorf("%w: length lock mismatch",ErrUntrusted)
 }
 b,err:=os.ReadFile(catalogPath)
 if err!=nil||len(b)!=CatalogSize{return empty,fmt.Errorf("%w: bundled catalog length mismatch",ErrUntrusted)}
 digest:=sha256.Sum256(b)
 hash:=hex.EncodeToString(digest[:])
 if hash!=CatalogSHA256{return empty,fmt.Errorf("%w: bundled catalog SHA-256 mismatch",ErrUntrusted)}
 doc,err:=v.ValidateBytes("catalog",b)
 if err!=nil{return empty,fmt.Errorf("%w: full schema/semantic validation failed",ErrUntrusted)}
 if doc["catalog_version"]!=CatalogVersion||doc["schema_version"]!=SchemaVersion||doc["kind"]!="neurometr.catalog"{
  return empty,fmt.Errorf("%w: catalog version or kind incompatible",ErrUntrusted)
 }
 // Validated version, hash and contract compatibility. No URL dereference.
 return Reference{
  Version:doc["catalog_version"].(string),
  SHA256:hash,
  Origin:"bundled",
  Compatibility:"passed",
 },nil
}
