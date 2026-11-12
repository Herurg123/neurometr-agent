//go:build windows

package main

import (
 "errors"
 "syscall"
 "unsafe"
)
func openBrowser(url string)error{
 // Windows ShellExecuteW opens only the loopback URL returned by our listener.
 // No user-supplied URL or catalog data are executed.
 verb,err:=syscall.UTF16PtrFromString("open");if err!=nil{return err}
 target,err:=syscall.UTF16PtrFromString(url);if err!=nil{return err}
 shell:=syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW")
 result,_,_:=shell.Call(0,uintptr(unsafe.Pointer(verb)),uintptr(unsafe.Pointer(target)),0,0,1)
 if result<=32{return errors.New("browser could not be opened")}
 return nil
}
