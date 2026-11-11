//go:build windows && amd64

package diagnostic

import (
 "context"
 "encoding/binary"
 "errors"
 "fmt"
 "runtime"
 "strings"
 "syscall"
 "time"
 "unicode"
 "unicode/utf16"
 "unsafe"
)

var (
 kernel = syscall.NewLazyDLL("kernel32.dll")
 ntdll = syscall.NewLazyDLL("ntdll.dll")
 dxgi = syscall.NewLazyDLL("dxgi.dll")
 rtlGetVersion = ntdll.NewProc("RtlGetVersion")
 memoryStatus = kernel.NewProc("GlobalMemoryStatusEx")
 cpuCount = kernel.NewProc("GetActiveProcessorCount")
 logicalInfo = kernel.NewProc("GetLogicalProcessorInformationEx")
 newDXGIFactory = dxgi.NewProc("CreateDXGIFactory1")
)

func NativeProbes() Probes { return Probes{
 OS: probeOS,CPU:probeCPU,RAM:probeRAM,GPU:probeGPU,
}}

type osVersionInfo struct{
 Size,Major,Minor,Build,Platform uint32
 CSDVersion [128]uint16
 ServicePackMajor,ServicePackMinor,SuiteMask uint16
 ProductType,Reserved byte
}
func probeOS(ctx context.Context)(map[string]any,error){
 if err:=ctx.Err();err!=nil{return nil,err}
 ver:=osVersionInfo{Size:uint32(unsafe.Sizeof(osVersionInfo{}))}
 r,_,_:=rtlGetVersion.Call(uintptr(unsafe.Pointer(&ver)))
 if r!=0||ver.Major==0 {return nil,errors.New("RtlGetVersion failed")}
 return map[string]any{
  "family":"windows","arch":"x64",
  "version":fmt.Sprintf("%d.%d.%d",ver.Major,ver.Minor,ver.Build),
  "evidence":evidence("detected","win32:RtlGetVersion"),
 },nil
}

// The registry contains the well-known CPU model label, not the computer name.
// This function never returns registry access paths or errors in a diagnostic.
func registryCPUName()string{
 const keyPath="HARDWARE"+string(rune(92))+"DESCRIPTION"+string(rune(92))+"System"+string(rune(92))+"CentralProcessor"+string(rune(92))+"0"
 path,err:=syscall.UTF16PtrFromString(keyPath)
 if err!=nil{return ""}
 var key syscall.Handle
 if err=syscall.RegOpenKeyEx(syscall.HKEY_LOCAL_MACHINE,path,0,syscall.KEY_READ,&key);err!=nil{return ""}
 defer syscall.RegCloseKey(key)
 name,err:=syscall.UTF16PtrFromString("ProcessorNameString")
 if err!=nil{return ""}
 var kind,size uint32
 if err=syscall.RegQueryValueEx(key,name,nil,&kind,nil,&size);err!=nil||size==0||size>4096{return ""}
 if kind!=syscall.REG_SZ && kind!=syscall.REG_EXPAND_SZ{return ""}
 raw:=make([]byte,size)
 if err=syscall.RegQueryValueEx(key,name,nil,&kind,&raw[0],&size);err!=nil{return ""}
 units:=make([]uint16,0,len(raw)/2)
 for i:=0;i+1<len(raw);i+=2{units=append(units,binary.LittleEndian.Uint16(raw[i:i+2]))}
 return safeModelName(string(utf16.Decode(units)))
}

// Hardware names are allowed; free-form strings that could contain local
// addresses, paths, e-mails or controls are dropped rather than logged.
func safeModelName(raw string)string{
 raw=strings.TrimSpace(strings.TrimRightFunc(raw,func(ch rune)bool{return ch==rune(0)}))
 if raw==""||len(raw)>180{return ""}
 low:=strings.ToLower(raw)
 for _,needle:=range []string{"://","/users/","/home/","password","token","secret"}{
  if strings.Contains(low,needle){return ""}
 }
 if strings.ContainsRune(raw,rune(92)){return ""}
 if strings.Contains(raw,"@")&& !strings.Contains(raw," @ "){return ""}
 for _,c:=range raw{
  if unicode.IsControl(c)||c==rune(0x2028)||c==rune(0x2029){return ""}
 }
 return raw
}

func physicalCoreCount()(int,error){
 // RelationProcessorCore is 0. Count entries; do not infer physical cores
 // by dividing logical threads by two on heterogeneous CPUs.
 var required uint32
 logicalInfo.Call(0,0,uintptr(unsafe.Pointer(&required)))
 if required<8||required>1024*1024{return 0,errors.New("processor information unavailable")}
 buffer:=make([]byte,required)
 r,_,_:=logicalInfo.Call(0,uintptr(unsafe.Pointer(&buffer[0])),uintptr(unsafe.Pointer(&required)))
 if r==0 {return 0,errors.New("GetLogicalProcessorInformationEx failed")}
 n:=0
 for offset:=0;offset+8<=int(required);{
  relation:=binary.LittleEndian.Uint32(buffer[offset:offset+4])
  size:=binary.LittleEndian.Uint32(buffer[offset+4:offset+8])
  if size<8||int(size)>int(required)-offset{return 0,errors.New("invalid processor record size")}
  if relation==0{n++}
  offset+=int(size)
 }
 if n<=0{return 0,errors.New("no processor core records")}
 return n,nil
}
func probeCPU(ctx context.Context)(map[string]any,error){
 if err:=ctx.Err();err!=nil{return nil,err}
 model:=registryCPUName()
 phys,err:=physicalCoreCount()
 logical,_,_:=cpuCount.Call(0xffff)
 if logical==0&&runtime.NumCPU()>0{logical=uintptr(runtime.NumCPU())}
 var name any=nil
 if model!=""{name=model}
 var cores any=nil
 if err==nil&&phys>0{cores=phys}
 var logicals any=nil
 if logical>0{logicals=int(logical)}
 if name==nil&&cores==nil&&logicals==nil{return nil,errors.New("CPU details unavailable")}
 e:=evidence("detected","win32:registry+GetLogicalProcessorInformationEx+GetActiveProcessorCount")
 if err!=nil{
  e=evidence("unknown","win32:GetLogicalProcessorInformationEx:unavailable")
 }
 return map[string]any{"model":name,"physical_cores":cores,"logical_processors":logicals,"evidence":e},nil
}

type memoryStatusEx struct {
 Size,MemoryLoad uint32
 TotalPhysical,AvailablePhysical uint64
 TotalPageFile,AvailablePageFile uint64
 TotalVirtual,AvailableVirtual,AvailableExtendedVirtual uint64
}
func safeByteCount(x uint64)any{
 if x>9007199254740991{return nil}
 return x
}
func probeRAM(ctx context.Context)(map[string]any,error){
 if err:=ctx.Err();err!=nil{return nil,err}
 status:=memoryStatusEx{Size:uint32(unsafe.Sizeof(memoryStatusEx{}))}
 r,_,_:=memoryStatus.Call(uintptr(unsafe.Pointer(&status)))
 if r==0||status.TotalPhysical==0{return nil,errors.New("GlobalMemoryStatusEx failed")}
 now:=time.Now().UTC().Format(time.RFC3339Nano)
 return map[string]any{
  "physical_total_bytes":safeByteCount(status.TotalPhysical),
  "available_bytes":safeByteCount(status.AvailablePhysical),
  "available_observed_at":now,
  "evidence":map[string]any{
   "kind":"measured","source":"win32:GlobalMemoryStatusEx","observed_at":now,
   "notes":"OS-visible physical memory, potentially excluding reserved firmware/UMA capacity.",
  },
 },nil
}

type dxgiGUID struct{D1 uint32;D2,D3 uint16;D4 [8]byte}
// IID_IDXGIFactory1 {770aae78-f26f-4dba-a829-253c83d1b387}
var dxgiFactoryIID=dxgiGUID{0x770aae78,0xf26f,0x4dba,[8]byte{0xa8,0x29,0x25,0x3c,0x83,0xd1,0xb3,0x87}}
type dxgiAdapterDesc1 struct {
 Description [128]uint16
 VendorID,DeviceID,SubSysID,Revision uint32
 DedicatedVideoMemory,DedicatedSystemMemory,SharedSystemMemory uintptr
 LUID int64
 Flags uint32
 _ uint32 // x64 alignment padding
}
// Vtable indices: IUnknown.Release 2, IDXGIFactory1.EnumAdapters1 12,
// IDXGIAdapter1.GetDesc1 10. This invokes stable native COM methods and
// never starts PowerShell, WMI or a graphics compute backend.
func comCall(p unsafe.Pointer,index int,args ...uintptr)uintptr{
 table:=*(*unsafe.Pointer)(p)
 fn:=*(*uintptr)(unsafe.Add(table,index*int(unsafe.Sizeof(uintptr(0)))))
 callArgs:=append([]uintptr{uintptr(p)},args...)
 res,_,_:=syscall.SyscallN(fn,callArgs...)
 return res
}
func hrFailed(hr uintptr)bool{return int32(uint32(hr))<0}
func probeGPU(ctx context.Context)(map[string]any,error){
 if err:=ctx.Err();err!=nil{return nil,err}
 if err:=newDXGIFactory.Find();err!=nil{return nil,errors.New("DXGI unavailable")}
 var factory unsafe.Pointer
 hr,_,_:=newDXGIFactory.Call(uintptr(unsafe.Pointer(&dxgiFactoryIID)),uintptr(unsafe.Pointer(&factory)))
 if hrFailed(hr)||factory==nil{return nil,errors.New("DXGI factory unavailable")}
 defer comCall(factory,2)
 var adapters []any
 adapters=make([]any,0)
 for i:=0;i<32;i++{
  if err:=ctx.Err();err!=nil{return nil,err}
  var adapter unsafe.Pointer
  hr=comCall(factory,12,uintptr(i),uintptr(unsafe.Pointer(&adapter)))
  if uint32(hr)==0x887a0002{break} // DXGI_ERROR_NOT_FOUND
  if hrFailed(hr)||adapter==nil{return nil,errors.New("DXGI adapter enumeration failed")}
  var desc dxgiAdapterDesc1
  status:=comCall(adapter,10,uintptr(unsafe.Pointer(&desc)))
  comCall(adapter,2)
  if hrFailed(status){return nil,errors.New("DXGI adapter description failed")}
  if desc.Flags&2!=0{continue} // DXGI_ADAPTER_FLAG_SOFTWARE
  name:=safeModelName(syscall.UTF16ToString(desc.Description[:]))
  var model any=nil
  if name!=""{model=name}
  dedicated:=uint64(desc.DedicatedVideoMemory)
  shared:=uint64(desc.SharedSystemMemory)
  var dedicatedBytes,sharedBytes any=nil,nil
  if dedicated>0{dedicatedBytes=safeByteCount(dedicated)}
  if shared>0{sharedBytes=safeByteCount(shared)}
  topology:="unknown"
  if dedicatedBytes!=nil{topology="dedicated"}
  if dedicatedBytes==nil&&sharedBytes!=nil{topology="shared"}
  adapters=append(adapters,map[string]any{
   "index":len(adapters),
   "name":model,
   "memory":map[string]any{
    "dedicated_total_bytes":dedicatedBytes,
    "dedicated_available_bytes":nil,
    "shared_total_bytes":sharedBytes,
   },
   "memory_topology":topology,
   "evidence":map[string]any{
    "kind":"detected","source":"win32:DXGI:IDXGIAdapter1.GetDesc1",
    "notes":"SharedSystemMemory is a maximum allocation ceiling, not additional installed RAM; no free VRAM was queried.",
   },
  })
 }
 return map[string]any{"adapters":adapters},nil
}
