// Package llmfit contains the limited A2 adapter to a pinned llmfit CLI.
// It does not implement diagnostics, catalog updates, networking or a UI.
package llmfit

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "fmt"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "time"

 "github.com/Herurg123/neurometr-agent/internal/contract"
)

const Version = "1.1.20"
const maxStdout = 2 * 1024 * 1024
const maxStderr = 256 * 1024
const maxBinary = 128 * 1024 * 1024

type Operation string

const (
 System Operation = "system"
 List Operation = "list"
 Fit Operation = "fit"
 FitProvider Operation = "fit_provider"
 Recommend Operation = "recommend"
 Search Operation = "search"
 Info Operation = "info"
)
type Failure string
const (
 Missing Failure = "missing"
 InvalidBinary Failure = "invalid_binary"
 VersionMismatch Failure = "version_mismatch"
 Timeout Failure = "timeout"
 ExitNonzero Failure = "nonzero_exit"
 InvalidJSON Failure = "invalid_json"
 UnexpectedShape Failure = "unexpected_json_shape"
 OutputTooLarge Failure = "output_too_large"
 InvalidRequest Failure = "invalid_request"
)
type CLIError struct {
 Kind Failure
 ExitCode int
 Err error
}
func (e *CLIError) Error() string {return fmt.Sprintf("llmfit %s (exit=%d): %v",e.Kind,e.ExitCode,e.Err)}
func (e *CLIError) Unwrap() error {return e.Err}

type Request struct {
 Executable string
 Operation Operation
 Selector string
 CustomModelsPath string
 ExpectedExecutableSHA256 string
 Timeout time.Duration
}
type Result struct {
 Version string
 Operation Operation
 Document any
 Stderr []byte
 Duration time.Duration
 ExitCode int
}
type limitedBuffer struct{
 buf bytes.Buffer
 limit int
 overflow bool
}
func (w *limitedBuffer) Len() int {return w.buf.Len()}
func (w *limitedBuffer) Bytes() []byte{return w.buf.Bytes()}
func (w *limitedBuffer) Write(p []byte)(int,error){
 if w.Len()+len(p)>w.limit {
  w.overflow=true
  rem:=w.limit-w.Len()
  if rem>0 {_,_=w.buf.Write(p[:rem])}
  return 0,errors.New("output cap exceeded")
 }
 return w.buf.Write(p)
}
func failure(kind Failure,exit int,err error)error{return &CLIError{Kind:kind,ExitCode:exit,Err:err}}
func verifyExecutable(path,sha string)error{
 if !filepath.IsAbs(path) {return failure(InvalidRequest,-1,errors.New("executable path must be absolute"))}
 info,err:=os.Lstat(path)
 if os.IsNotExist(err){return failure(Missing,-1,err)}
 if err!=nil{return failure(InvalidBinary,-1,err)}
 if !info.Mode().IsRegular()||info.Size()<=0||info.Size()>maxBinary{return failure(InvalidBinary,-1,errors.New("executable is not a regular, bounded file"))}
 if sha!=""{
  if len(sha)!=64 {return failure(InvalidRequest,-1,errors.New("invalid expected binary sha256"))}
  f,err:=os.Open(path);if err!=nil{return failure(InvalidBinary,-1,err)};defer f.Close()
  h:=sha256.New();if _,err=io.Copy(h,io.LimitReader(f,maxBinary+1));err!=nil{return failure(InvalidBinary,-1,err)}
  if hex.EncodeToString(h.Sum(nil))!=strings.ToLower(sha){return failure(InvalidBinary,-1,errors.New("binary SHA-256 does not match pin"))}
 }
 return nil
}
func cleanedEnv(custom,exe string)([]string,error){
 // No GITHUB_TOKEN/GH_TOKEN, proxy credentials, provider API keys or arbitrary LLMFIT_*.
 keep:=map[string]bool{
  "PATH":true,"SYSTEMROOT":true,"WINDIR":true,"COMSPEC":true,
  "TEMP":true,"TMP":true,"APPDATA":true,"LOCALAPPDATA":true,
  "USERPROFILE":true,"HOME":true,"LANG":true,
 }
 if strings.HasPrefix(strings.ToLower(filepath.Base(exe)),"llmfitmock") {keep["NEUROMETR_MOCK_MODE"]=true}
 result:=[]string{}
 for _,entry:=range os.Environ(){
  k,_,ok:=strings.Cut(entry,"=");if ok&&keep[strings.ToUpper(k)] {result=append(result,entry)}
 }
 if custom!=""{
  if !filepath.IsAbs(custom){return nil,failure(InvalidRequest,-1,errors.New("custom model path must be absolute"))}
  s,err:=os.Lstat(custom);if err!=nil{return nil,failure(InvalidRequest,-1,err)}
  if !s.Mode().IsRegular()||s.Size()>1024*1024{return nil,failure(InvalidRequest,-1,errors.New("custom model input must be a file <=1MiB"))}
  result=append(result,"LLMFIT_CUSTOM_MODELS="+custom)
 } else {
  // A missing path disables accidental use of a user's existing custom overlay.
  result=append(result,"LLMFIT_CUSTOM_MODELS="+filepath.Join(os.TempDir(),"neurometr-a2-no-custom-models.json"))
 }
 return result,nil
}
func launch(ctx context.Context,exe string,args []string,custom string)([]byte,[]byte,time.Duration,int,error){
 env,err:=cleanedEnv(custom,exe);if err!=nil{return nil,nil,0,-1,err}
 cmd:=exec.CommandContext(ctx,exe,args...)
 cmd.Env=env
 cmd.Dir=filepath.Dir(exe)
 stdout:=&limitedBuffer{limit:maxStdout}
 stderr:=&limitedBuffer{limit:maxStderr}
 cmd.Stdout=stdout;cmd.Stderr=stderr
 start:=time.Now()
 err=cmd.Run();elapsed:=time.Since(start)
 code:=0
 if cmd.ProcessState!=nil{code=cmd.ProcessState.ExitCode()}
 if ctx.Err()==context.DeadlineExceeded{return stdout.Bytes(),stderr.Bytes(),elapsed,code,failure(Timeout,code,ctx.Err())}
 if ctx.Err()!=nil{return stdout.Bytes(),stderr.Bytes(),elapsed,code,failure(Timeout,code,ctx.Err())}
 if stdout.overflow||stderr.overflow{return stdout.Bytes(),stderr.Bytes(),elapsed,code,failure(OutputTooLarge,code,errors.New("bounded output exceeded"))}
 if err!=nil{
  if errors.Is(err,exec.ErrNotFound)||os.IsNotExist(err){return stdout.Bytes(),stderr.Bytes(),elapsed,code,failure(Missing,code,err)}
  return stdout.Bytes(),stderr.Bytes(),elapsed,code,failure(ExitNonzero,code,err)
 }
 return stdout.Bytes(),stderr.Bytes(),elapsed,code,nil
}

func checkVersion(ctx context.Context,exe string)(string,error){
 out,_,_,code,err:=launch(ctx,exe,[]string{"--version"},"")
 if err!=nil{return "",err}
 s:=strings.TrimSpace(string(out))
 valid:=strings.Contains(strings.ToLower(s),"llmfit")
 exact:=false
 for _,part:=range strings.Fields(s){if part==Version{exact=true}}
 if !valid||!exact{return s,failure(VersionMismatch,code,fmt.Errorf("expected llmfit %s",Version))}
 return s,nil
}
func ProbeVersion(ctx context.Context,exe string,timeout time.Duration)(string,error){
 if err:=verifyExecutable(exe,"");err!=nil{return "",err}
 if timeout<=0||timeout>60*time.Second{timeout=10*time.Second}
 c,cancel:=context.WithTimeout(ctx,timeout);defer cancel()
 return checkVersion(c,exe)
}

func Run(ctx context.Context,r Request)(Result,error){
 result:=Result{Operation:r.Operation,ExitCode:-1}
 if err:=verifyExecutable(r.Executable,r.ExpectedExecutableSHA256);err!=nil{return result,err}
 if r.Timeout<=0||r.Timeout>120*time.Second {r.Timeout=30*time.Second}
 var args []string
 switch r.Operation{
 case System:args=[]string{"--no-dashboard","--json","system"}
 case List:args=[]string{"--no-dashboard","--json","list"}
 case Fit:args=[]string{"--no-dashboard","--json","fit","-n","5"}
 case FitProvider:
  if r.Selector==""||len(r.Selector)>64||strings.HasPrefix(r.Selector,"-")||strings.ContainsAny(r.Selector,"\r\n\x00"){return result,failure(InvalidRequest,-1,errors.New("invalid provider"))}
  args=[]string{"--no-dashboard","--json","fit","--show-incompatible","--providers",r.Selector,"-n","10"}
 case Recommend:args=[]string{"--no-dashboard","--json","recommend","--limit","5"}
 case Search,Info:
  if r.Selector==""||len(r.Selector)>128||strings.HasPrefix(r.Selector,"-")||strings.ContainsAny(r.Selector,string([]rune{10,13,0})){return result,failure(InvalidRequest,-1,errors.New("invalid selector"))}
  args=[]string{"--no-dashboard","--json",string(r.Operation),r.Selector}
 default:return result,failure(InvalidRequest,-1,errors.New("operation is not allowlisted"))
 }
 c,cancel:=context.WithTimeout(ctx,r.Timeout);defer cancel()
 version,err:=checkVersion(c,r.Executable);if err!=nil{return result,err};result.Version=version
 stdout,stderr,elapsed,code,err:=launch(c,r.Executable,args,r.CustomModelsPath)
 result.Duration=elapsed;result.ExitCode=code;result.Stderr=stderr
 if err!=nil{return result,err}
 doc,err:=contract.DecodeBounded(stdout,maxStdout)
 if err!=nil{return result,failure(InvalidJSON,code,err)}
 switch r.Operation{
 case List,Search:
  if _,ok:=doc.([]any);!ok{return result,failure(UnexpectedShape,code,errors.New("expected model array"))}
 default:
  m,ok:=doc.(map[string]any)
  if !ok{return result,failure(UnexpectedShape,code,errors.New("expected JSON object"))}
  if _,ok=m["system"].(map[string]any);!ok{return result,failure(UnexpectedShape,code,errors.New("missing system object"))}
  if r.Operation!=System{
   if _,ok=m["models"].([]any);!ok{return result,failure(UnexpectedShape,code,errors.New("missing models array"))}
  }
 }
 result.Document=doc
 return result,nil
}
