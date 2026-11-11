package llmfit

import (
 "context"
 "errors"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "testing"
 "time"
)

func compileMock(t *testing.T)string{
 t.Helper()
 suffix:="";if runtime.GOOS=="windows"{suffix=".exe"}
 output:=filepath.Join(t.TempDir(),"llmfitmock"+suffix)
 c:=exec.Command("go","build","-o",output,"./testdata/mockcli")
 if b,e:=c.CombinedOutput();e!=nil{t.Fatalf("build synthetic mock CLI: %v\n%s",e,b)}
 return output
}
func wantKind(t *testing.T,err error,kind Failure){
 t.Helper()
 var e *CLIError
 if !errors.As(err,&e)||e.Kind!=kind{t.Fatalf("expected %s, got %v",kind,err)}
}
func TestControlledSubprocessMock(t *testing.T){
 exe:=compileMock(t)
 good:=Request{Executable:exe,Operation:System,Timeout:3*time.Second}
 t.Run("system_and_version",func(t *testing.T){
  t.Setenv("NEUROMETR_MOCK_MODE","ok")
  result,err:=Run(context.Background(),good)
  if err!=nil{t.Fatal(err)}
  if result.Version!="llmfit 1.1.20"||result.Document==nil||result.ExitCode!=0{t.Fatalf("bad successful subprocess outcome %+v",result)}
 })
 t.Run("list_array",func(t *testing.T){
  t.Setenv("NEUROMETR_MOCK_MODE","ok")
  r:=good;r.Operation=List
  result,err:=Run(context.Background(),r);if err!=nil{t.Fatal(err)}
  if len(result.Document.([]any))!=1{t.Fatal("missing model list")}
 })
 t.Run("search_selector_argument",func(t *testing.T){
  t.Setenv("NEUROMETR_MOCK_MODE","ok")
  r:=good;r.Operation=Search;r.Selector="Model-1B"
  if _,err:=Run(context.Background(),r);err!=nil{t.Fatal(err)}
 })
 t.Run("selector_injection_rejected",func(t *testing.T){
  r:=good;r.Operation=Info;r.Selector="--share"
  _,err:=Run(context.Background(),r);wantKind(t,err,InvalidRequest)
 })
 t.Run("missing_binary",func(t *testing.T){
  r:=good;r.Executable=filepath.Join(t.TempDir(),"does-not-exist.exe")
  _,err:=Run(context.Background(),r);wantKind(t,err,Missing)
 })
 t.Run("wrong_hash_rejected",func(t *testing.T){
  r:=good;r.ExpectedExecutableSHA256=strings.Repeat("0",64)
  _,err:=Run(context.Background(),r);wantKind(t,err,InvalidBinary)
 })
 t.Run("bad_version",func(t *testing.T){
  t.Setenv("NEUROMETR_MOCK_MODE","bad_version")
  _,err:=Run(context.Background(),good);wantKind(t,err,VersionMismatch)
 })
 t.Run("nonzero_exit_and_code",func(t *testing.T){
  t.Setenv("NEUROMETR_MOCK_MODE","nonzero")
  result,err:=Run(context.Background(),good);wantKind(t,err,ExitNonzero)
  if result.ExitCode!=19{t.Fatalf("wrong exit %d",result.ExitCode)}
 })
 t.Run("invalid_json",func(t *testing.T){
  t.Setenv("NEUROMETR_MOCK_MODE","malformed")
  _,err:=Run(context.Background(),good);wantKind(t,err,InvalidJSON)
 })
 t.Run("trailing_json",func(t *testing.T){
  t.Setenv("NEUROMETR_MOCK_MODE","trailing")
  _,err:=Run(context.Background(),good);wantKind(t,err,InvalidJSON)
 })
 t.Run("wrong_shape",func(t *testing.T){
  t.Setenv("NEUROMETR_MOCK_MODE","wrong_shape")
  _,err:=Run(context.Background(),good);wantKind(t,err,UnexpectedShape)
 })
 t.Run("timeout",func(t *testing.T){
  t.Setenv("NEUROMETR_MOCK_MODE","sleep")
  r:=good;r.Timeout=250*time.Millisecond
  start:=time.Now()
  _,err:=Run(context.Background(),r);wantKind(t,err,Timeout)
  if time.Since(start)>5*time.Second{t.Fatal("timeout did not bound execution")}
 })
 t.Run("oversized_stdout",func(t *testing.T){
  t.Setenv("NEUROMETR_MOCK_MODE","too_large")
  _,err:=Run(context.Background(),good);wantKind(t,err,OutputTooLarge)
 })
 t.Run("stderr_warning_not_failure",func(t *testing.T){
  t.Setenv("NEUROMETR_MOCK_MODE","warning")
  result,err:=Run(context.Background(),good)
  if err!=nil||len(result.Stderr)==0{t.Fatalf("warning caused failure or was discarded: %v",err)}
 })
 t.Run("secret_env_stripped",func(t *testing.T){
  t.Setenv("NEUROMETR_MOCK_MODE","environment")
  t.Setenv("GITHUB_TOKEN","test-secret-never-export")
  r:=good
  overlay:=filepath.Join(t.TempDir(),"custom.json")
  if err:=os.WriteFile(overlay,[]byte("[]"),0600);err!=nil{t.Fatal(err)}
  r.CustomModelsPath=overlay
  result,err:=Run(context.Background(),r);if err!=nil{t.Fatal(err)}
  doc:=result.Document.(map[string]any)["system"].(map[string]any)
  if doc["token_leaked"]!=false||doc["custom_path"]!=overlay{t.Fatalf("environment isolation failed: %v",doc)}
 })
 t.Run("partial_error_exposes_stderr_and_exit",func(t *testing.T){
  t.Setenv("NEUROMETR_MOCK_MODE","failed_json")
  r,err:=Run(context.Background(),good);wantKind(t,err,ExitNonzero)
  if r.ExitCode!=11||len(r.Stderr)==0{t.Fatal("lost failure context")}
 })
}
