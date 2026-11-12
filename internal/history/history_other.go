//go:build !windows

package history

import (
 "os"
 "path/filepath"
)

func ensurePrivateDirectory(path string)error{
 info,err:=os.Lstat(path)
 if err!=nil||!info.IsDir()||info.Mode()&os.ModeSymlink!=0{return ErrUnavailable}
 if info.Mode().Perm()&0077!=0{return ErrUnavailable}
 return nil
}
func moveNoReplace(src,dst string)error{
 // The temporary file and final report share one directory/filesystem.
 // Link(2) fails if the destination already exists, avoiding replacement.
 if err:=os.Link(src,dst);err!=nil{return err}
 if err:=os.Remove(src);err!=nil{return err}
 return nil
}
var _=filepath.Separator
