//go:build windows

package history

import (
 "errors"
 "path/filepath"
 "syscall"
 "unsafe"
)

// Windows security is verified using native ACLs, never inferred from Go mode
// bits. If we cannot examine the DACL, history fails closed.
var (
 advapi= syscall.NewLazyDLL("advapi32.dll")
 kernel= syscall.NewLazyDLL("kernel32.dll")
 getNamedInfo=advapi.NewProc("GetNamedSecurityInfoW")
 getAce=advapi.NewProc("GetAce")
 sidToString=advapi.NewProc("ConvertSidToStringSidW")
 openProcessToken=advapi.NewProc("OpenProcessToken")
 getTokenInfo=advapi.NewProc("GetTokenInformation")
 getCurrentProcess=kernel.NewProc("GetCurrentProcess")
 localFree=kernel.NewProc("LocalFree")
 fileAttributes=kernel.NewProc("GetFileAttributesW")
 moveFile=kernel.NewProc("MoveFileW")
)
const (
 fileAttributeReparse=0x400
 fileAttributeDirectory=0x10
 seFileObject=1
 daclSecurityInformation=4
 tokenQuery=0x0008
 tokenUserClass=1
)
func utf16(s string)*uint16{
 p,err:=syscall.UTF16PtrFromString(s)
 if err!=nil{return nil}
 return p
}
func sidString(sid uintptr)(string,error){
 if sid==0{return "",ErrUnavailable}
 var value unsafe.Pointer
 r,_,_:=sidToString.Call(sid,uintptr(unsafe.Pointer(&value)))
 if r==0||value==nil{return "",ErrUnavailable}
 defer localFree.Call(uintptr(value))
 // ConvertSidToStringSidW always returns a short NUL-terminated string.
 // Do not expose its raw pointer or any SID in diagnostic HTTP/log output.
 chars:=unsafe.Slice((*uint16)(value),256)
 for i,ch:=range chars{
  if ch==0{return syscall.UTF16ToString(chars[:i]),nil}
 }
 return "",ErrUnavailable
}
func currentSID()(string,error){
 proc,_,_:=getCurrentProcess.Call()
 var handle uintptr
 ok,_,_:=openProcessToken.Call(proc,tokenQuery,uintptr(unsafe.Pointer(&handle)))
 if ok==0{return "",ErrUnavailable}
 defer syscall.CloseHandle(syscall.Handle(handle))
 var n uint32
 getTokenInfo.Call(handle,tokenUserClass,0,0,uintptr(unsafe.Pointer(&n)))
 if n<uint32(unsafe.Sizeof(uintptr(0)))||n>4096{return "",ErrUnavailable}
 b:=make([]byte,n)
 ok,_,_ = getTokenInfo.Call(handle,tokenUserClass,uintptr(unsafe.Pointer(&b[0])),uintptr(n),uintptr(unsafe.Pointer(&n)))
 if ok==0{return "",ErrUnavailable}
 sid:=*(*uintptr)(unsafe.Pointer(&b[0]))
 return sidString(sid)
}
// Windows ACL masks can contain FILE_GENERIC_READ, GENERIC_READ and
// inherited Windows access flags. We reject *all* allow ACEs to any principal
// except the current owner, LocalSystem, Administrators and owner/creator.
func ensurePrivateDirectory(path string)error{
 for _,p:=range []string{filepath.Dir(path),path}{
  at:=utf16(p)
  if at==nil{return ErrUnavailable}
  flags,_,_:=fileAttributes.Call(uintptr(unsafe.Pointer(at)))
  if uint32(flags)==0xffffffff||uint32(flags)&fileAttributeDirectory==0||
    uint32(flags)&fileAttributeReparse!=0{return ErrUnavailable}
 }
 me,err:=currentSID()
 if err!=nil{return ErrUnavailable}
 pathPtr:=utf16(path)
 var dacl,desc uintptr
 status,_,_:=getNamedInfo.Call(
  uintptr(unsafe.Pointer(pathPtr)),seFileObject,daclSecurityInformation,
  0,0,uintptr(unsafe.Pointer(&dacl)),0,uintptr(unsafe.Pointer(&desc)),
 )
 if status!=0||desc==0{return ErrUnavailable}
 defer localFree.Call(desc)
 // A NULL DACL grants unrestricted access to everybody.
 if dacl==0{return ErrUnavailable}
 type aclHeader struct{ Revision,Reserved byte; Size,Count,Reserved2 uint16 }
 hdr:=(*aclHeader)(unsafe.Pointer(dacl))
 if hdr.Size<8||hdr.Count>4096{return ErrUnavailable}
 allowed:=map[string]bool{
  me:true,
  "S-1-5-18":true, // SYSTEM
  "S-1-5-32-544":true, // Administrators
  "S-1-3-0":true, // CREATOR OWNER
  "S-1-3-4":true, // OWNER RIGHTS
 }
 for i:=0;i<int(hdr.Count);i++{
  var ptr uintptr
  ok,_,_:=getAce.Call(dacl,uintptr(i),uintptr(unsafe.Pointer(&ptr)))
  if ok==0||ptr==0{return ErrUnavailable}
  t:=*(*byte)(unsafe.Pointer(ptr))
  if t==1||t==6{continue} // deny access ACE
  if t!=0{ // unknown allow/object ACE type: refuse to make ACL guarantees
   return ErrUnavailable
  }
  sid,err:=sidString(ptr+8) // ACCESS_ALLOWED_ACE SidStart
  if err!=nil||!allowed[sid]{return ErrUnavailable}
 }
 return nil
}
func moveNoReplace(src,dst string)error{
 sp,dp:=utf16(src),utf16(dst)
 if sp==nil||dp==nil{return ErrUnavailable}
 ok,_,e:=moveFile.Call(uintptr(unsafe.Pointer(sp)),uintptr(unsafe.Pointer(dp)))
 if ok==0{
  if !errors.Is(e,syscall.Errno(0)){return e}
  return ErrUnavailable
 }
 return nil
}
