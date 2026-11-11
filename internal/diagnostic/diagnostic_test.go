package diagnostic

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "testing"
 "time"

 "github.com/Herurg123/neurometr-agent/internal/contract"
)
func repoRoot(t *testing.T)string{t.Helper();p,e:=filepath.Abs(filepath.Join("..",".."));if e!=nil{t.Fatal(e)};return p}
func testCatalog(t *testing.T)Options{
 t.Helper()
 t.Setenv("NEUROMETR_A3_TEST_ONLY","1")
 root:=repoRoot(t)
 return Options{ContractRoot:root,CatalogPath:filepath.Join(root,"contracts","fixtures","catalog_valid.json"),AllowTestCatalog:true}
}
func fakeProbes()Probes{
 return Probes{
  OS:func(context.Context)(map[string]any,error){return map[string]any{
   "family":"windows","version":"10.0.22631","arch":"x64","evidence":evidence("detected","test:win32:os"),
  },nil},
  CPU:func(context.Context)(map[string]any,error){return map[string]any{
   "model":"Synthetic Test CPU","physical_cores":4,"logical_processors":8,"evidence":evidence("detected","test:win32:cpu"),
  },nil},
  RAM:func(context.Context)(map[string]any,error){return map[string]any{
   "physical_total_bytes":int64(16*1024*1024*1024),
   "available_bytes":int64(7*1024*1024*1024),
   "available_observed_at":"2026-11-11T10:00:00Z",
   "evidence":evidence("measured","test:win32:GlobalMemoryStatusEx"),
  },nil},
  GPU:func(context.Context)(map[string]any,error){return map[string]any{"adapters":[]any{
   map[string]any{"index":0,"name":"Synthetic GPU","memory":map[string]any{
    "dedicated_total_bytes":int64(4*1024*1024*1024),
    "dedicated_available_bytes":nil,
    "shared_total_bytes":int64(8*1024*1024*1024),
   },"memory_topology":"dedicated","evidence":evidence("detected","test:dxgi")},
  }},nil},
 }
}
func decodeReport(t *testing.T,raw []byte)map[string]any{
 t.Helper()
 parsed,err:=contract.DecodeBounded(raw,contract.DiagnosticLimit)
 if err!=nil{t.Fatal(err)}
 return parsed.(map[string]any)
}
func testGenerate(t *testing.T,opts Options,probes Probes)Result{
 t.Helper()
 out,err:=Generate(context.Background(),opts,probes)
 if err!=nil{t.Fatal(err)}
 validator,err:=contract.NewFullValidator(opts.ContractRoot)
 if err!=nil{t.Fatal(err)}
 if _,err=validator.ValidateBytes("diagnostic",out.JSON);err!=nil{t.Fatalf("generated JSON failed full validator: %v",err)}
 if len(out.JSON)>contract.DiagnosticLimit{t.Fatal("report exceeds 2MiB")}
 return out
}
func TestHardwareFirstValidOfflineNoLLMFit(t *testing.T){
 opts:=testCatalog(t)
 first:=testGenerate(t,opts,fakeProbes())
 second:=testGenerate(t,opts,fakeProbes())
 a,b:=decodeReport(t,first.JSON),decodeReport(t,second.JSON)
 if a["kind"]!="neurometr.diagnostic"||a["schema_version"]!="0.1.0"||a["overall_status"]!="ok"{t.Fatalf("invalid nominal status: %v",a["overall_status"])}
 if a["run_id"]==b["run_id"]{t.Fatal("run_id repeated")}
 if len(a["errors"].([]any))!=0||len(a["cloud_checks"].([]any))!=0||len(a["runtime_checks"].([]any))!=0||len(a["model_assessments"].([]any))!=0{t.Fatal("offline report gained network checks/model claims")}
 components:=a["components"].([]any)
 if components[1].(map[string]any)["status"]!="skipped"||components[2].(map[string]any)["status"]!="skipped"{t.Fatal("absent llmfit or cloud was not skipped")}
 if a["catalog_ref"].(map[string]any)["origin"]!="cached"{t.Fatal("test catalog origin not grounded in file")}
 bts,err:=os.ReadFile(opts.CatalogPath);if err!=nil{t.Fatal(err)}
 digest:=sha256.Sum256(bts)
 if a["catalog_ref"].(map[string]any)["sha256"]!=hex.EncodeToString(digest[:]){t.Fatal("catalog digest not computed from exact bytes")}
 gpu:=a["hardware"].(map[string]any)["gpus"].([]any)[0].(map[string]any)
 memory:=gpu["memory"].(map[string]any)
 if memory["dedicated_available_bytes"]!=nil{t.Fatal("unmeasured free VRAM invented")}
 if len(a["hardware"].(map[string]any)["npus"].([]any))!=0{t.Fatal("fabricated NPU")}
 if !bytes.Contains(first.JSON,[]byte("TEST ONLY:")){t.Fatal("synthetic test catalog not disclosed")}
 if first.HardwareElapsed>5*time.Second{t.Fatalf("synthetic hardware route slow: %s",first.HardwareElapsed)}
}
func TestCatalogRequiredAndNoFakeFallback(t *testing.T){
 opts:=testCatalog(t)
 opts.CatalogPath=""
 _,err:=Generate(context.Background(),opts,fakeProbes())
 if !errors.Is(err,ErrCatalogRequired){t.Fatalf("missing catalog not explicitly blocked: %v",err)}
 opts=testCatalog(t)
 opts.AllowTestCatalog=false
 if _,err=Generate(context.Background(),opts,fakeProbes());err==nil{t.Fatal("synthetic fixture accepted as production catalog")}
 opts=testCatalog(t)
 opts.CatalogPath=filepath.Join(t.TempDir(),"missing.json")
 if _,err=Generate(context.Background(),opts,fakeProbes());err==nil{t.Fatal("missing local catalog accepted")}
 opts=testCatalog(t)
 opts.CatalogPath=filepath.Join(t.TempDir(),"bad.json")
 if err=os.WriteFile(opts.CatalogPath,[]byte(`{"catalog_version":"fake","models":[]}`),0600);err!=nil{t.Fatal(err)}
 if _,err=Generate(context.Background(),opts,fakeProbes());err==nil{t.Fatal("invalid catalog accepted")}
}
func TestPartialFailuresRetainHardware(t *testing.T){
 cases:=[]struct{name string;edit func(*Probes)}{
  {"missing_gpu",func(p *Probes){p.GPU=func(context.Context)(map[string]any,error){return map[string]any{"adapters":[]any{}},nil}}},
  {"unknown_vram",func(p *Probes){p.GPU=func(context.Context)(map[string]any,error){return map[string]any{"adapters":[]any{map[string]any{
   "index":0,"name":"Unknown integrated GPU",
   "memory":map[string]any{"dedicated_total_bytes":nil,"dedicated_available_bytes":nil,"shared_total_bytes":nil},
   "memory_topology":"unknown","evidence":evidence("unknown","test:dxgi"),
  }}},nil}}},
  {"gpu_failed",func(p *Probes){p.GPU=func(context.Context)(map[string]any,error){return nil,errors.New("simulated hardware failure")}}},
  {"ram_failed",func(p *Probes){p.RAM=func(context.Context)(map[string]any,error){return nil,errors.New("simulated hardware failure")}}},
  {"slow_gpu_probe",func(p *Probes){p.GPU=func(ctx context.Context)(map[string]any,error){<-ctx.Done();return nil,ctx.Err()}}},
  {"all_hardware_failed",func(p *Probes){fail:=func(context.Context)(map[string]any,error){return nil,errors.New("probe failure")};p.OS=fail;p.CPU=fail;p.RAM=fail;p.GPU=fail}},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   opts:=testCatalog(t)
   opts.PerProbeTimeout=30*time.Millisecond
   p:=fakeProbes();tc.edit(&p)
   out:=testGenerate(t,opts,p)
   d:=decodeReport(t,out.JSON)
   hardware:=d["hardware"].(map[string]any)
   ram:=hardware["ram"].(map[string]any)
   if tc.name!="ram_failed"&&tc.name!="all_hardware_failed"&&ram["physical_total_bytes"]==nil{t.Fatal("previous RAM lost on partial failure")}
   if len(d["cloud_checks"].([]any))!=0{t.Fatal("offline run created cloud checks")}
   switch tc.name{
   case "missing_gpu","unknown_vram":
    if d["overall_status"]!="ok"{t.Fatal("optional unknown GPU cannot invalidate detected CPU and RAM")}
    if len(d["errors"].([]any))!=0{t.Fatal("unknown topology erroneously classified as failure")}
   case "all_hardware_failed":
    if d["overall_status"]!="failed"{t.Fatal("zero useful data must be failed")}
   default:
    if d["overall_status"]!="partial"{t.Fatalf("wanted partial, got %v",d["overall_status"])}
    if len(d["errors"].([]any))==0{t.Fatal("structured error missing")}
   }
  })
 }
}
func TestExplicitLLMFitOptionalFailures(t *testing.T){
 opts:=testCatalog(t)
 opts.LLMFitExecutable=filepath.Join(t.TempDir(),"missing-llmfit.exe")
 out:=testGenerate(t,opts,fakeProbes())
 doc:=decodeReport(t,out.JSON)
 if doc["overall_status"]!="partial"||out.HardwareStatus!="ok"||out.LLMFitStatus!="failed"{t.Fatal("optional missing CLI dropped useful hardware")}
 errs:=doc["errors"].([]any)
 if len(errs)!=1||errs[0].(map[string]any)["code"]!="LLMFIT_MISSING"{t.Fatalf("missing CLI error: %v",errs)}
 if doc["hardware"].(map[string]any)["ram"].(map[string]any)["physical_total_bytes"]==nil{t.Fatal("hardware erased on CLI failure")}
 // No external executable has to be installed in either mock runner.
 suffix:="";if runtime.GOOS=="windows"{suffix=".exe"}
 mock:=filepath.Join(t.TempDir(),"llmfitmock"+suffix)
 b,err:=exec.Command("go","build","-o",mock,"../llmfit/testdata/mockcli").CombinedOutput()
 if err!=nil{t.Fatalf("mock build failed %v: %s",err,b)}
 t.Setenv("NEUROMETR_MOCK_MODE","sleep")
 opts.LLMFitExecutable=mock;opts.LLMFitTimeout=200*time.Millisecond
 out=testGenerate(t,opts,fakeProbes())
 d:=decodeReport(t,out.JSON)
 if d["overall_status"]!="partial"||d["errors"].([]any)[0].(map[string]any)["code"]!="LLMFIT_TIMEOUT"{t.Fatal("CLI timeout not represented as partial")}
 t.Setenv("NEUROMETR_MOCK_MODE","malformed")
 opts.LLMFitTimeout=2*time.Second
 out=testGenerate(t,opts,fakeProbes())
 d=decodeReport(t,out.JSON)
 if d["errors"].([]any)[0].(map[string]any)["code"]!="LLMFIT_INVALID_JSON"{t.Fatal("malformed JSON not classified")}
 t.Setenv("NEUROMETR_MOCK_MODE","nonzero")
 out=testGenerate(t,opts,fakeProbes())
 d=decodeReport(t,out.JSON)
 if d["errors"].([]any)[0].(map[string]any)["code"]!="LLMFIT_EXIT_NONZERO"{t.Fatal("nonzero exit not classified")}
 t.Setenv("NEUROMETR_MOCK_MODE","ok")
 out=testGenerate(t,opts,fakeProbes())
 d=decodeReport(t,out.JSON)
 if d["overall_status"]!="ok"||out.LLMFitStatus!="ok"{t.Fatal("successful optional CLI not recorded")}
 if len(d["model_assessments"].([]any))!=0||len(d["runtime_checks"].([]any))!=0{t.Fatal("CLI system probe was confused with model execution")}
}
func TestPIIIsNotEmitted(t *testing.T){
 opts:=testCatalog(t)
 p:=fakeProbes()
 p.CPU=func(context.Context)(map[string]any,error){return map[string]any{
  "model":"private@example.com", "physical_cores":4,"logical_processors":8,
  "evidence":evidence("detected","test:cpu"),
 },nil}
 p.GPU=func(context.Context)(map[string]any,error){return map[string]any{"adapters":[]any{map[string]any{
  "index":0,"name":"C:"+string(rune(92))+"Users"+string(rune(92))+"PrivateUser"+string(rune(92))+"adapter",
  "memory":map[string]any{"dedicated_total_bytes":nil,"dedicated_available_bytes":nil,"shared_total_bytes":nil},
  "memory_topology":"unknown","evidence":evidence("unknown","test:gpu"),
 }}},nil}
 out:=testGenerate(t,opts,p)
 for _,s:=range []string{"private@example.com","C:"+string(rune(92))+"Users"+string(rune(92))+"PrivateUser","username","hostname","serial_number","mac_address","ip_address","GITHUB_TOKEN","USERPROFILE"}{
  if bytes.Contains(bytes.ToLower(out.JSON),bytes.ToLower([]byte(s))){t.Fatalf("sensitive string %q leaked into JSON",s)}
 }
 h:=decodeReport(t,out.JSON)["hardware"].(map[string]any)
 if h["cpu"].(map[string]any)["model"]!=nil||h["gpus"].([]any)[0].(map[string]any)["name"]!=nil{t.Fatal("untrusted device model strings were not suppressed")}
}
func TestLargeOutputNoUnsafeReport(t *testing.T){
 opts:=testCatalog(t)
 p:=fakeProbes()
 p.GPU=func(context.Context)(map[string]any,error){
  list:=make([]any,11000)
  for i:=range list{list[i]=map[string]any{
   "index":i,"name":"Synthetic GPU",
   "memory":map[string]any{"dedicated_total_bytes":nil,"dedicated_available_bytes":nil,"shared_total_bytes":nil},
   "memory_topology":"unknown","evidence":evidence("unknown","test:dxgi"),
  }}
  return map[string]any{"adapters":list},nil
 }
 result,err:=Generate(context.Background(),opts,p)
 if err==nil||len(result.JSON)!=0{t.Fatal("oversized generated report was emitted")}
 if !strings.Contains(err.Error(),"2 MiB"){t.Fatalf("unexpected size gate failure: %v",err)}
}
func TestFullValidatorRejectsMutatedRealOutput(t *testing.T){
 opts:=testCatalog(t)
 good:=testGenerate(t,opts,fakeProbes())
 d:=decodeReport(t,good.JSON)
 d["overall_status"]="failed" // hardware succeeded, contradictory
 bad,err:=json.Marshal(d);if err!=nil{t.Fatal(err)}
 v,err:=contract.NewFullValidator(opts.ContractRoot);if err!=nil{t.Fatal(err)}
 if _,err=v.ValidateBytes("diagnostic",bad);err==nil{t.Fatal("semantic contradiction accepted")}
}

 
// An external copy of catalog_valid.json is still synthetic. Neither a matching
// SHA-256 nor schema compliance supplies the unapproved production trust anchor.
func TestCopiedCatalogCannotBypassProductionGate(t *testing.T){
 opts:=testCatalog(t)
 raw,err:=os.ReadFile(opts.CatalogPath)
 if err!=nil{t.Fatal(err)}
 copied:=filepath.Join(t.TempDir(),"catalog-copy.json")
 if err=os.WriteFile(copied,raw,0600);err!=nil{t.Fatal(err)}
 validator,err:=contract.NewFullValidator(opts.ContractRoot)
 if err!=nil{t.Fatal(err)}
 if _,err=validator.ValidateBytes("catalog",raw);err!=nil{
  t.Fatalf("precondition: synthetic catalog must be structurally valid: %v",err)
 }
 originalDigest:=sha256.Sum256(raw)
 copiedBytes,err:=os.ReadFile(copied);if err!=nil{t.Fatal(err)}
 if sha256.Sum256(copiedBytes)!=originalDigest{t.Fatal("copied bytes differ from fixture")}
 opts.CatalogPath=copied
 opts.AllowTestCatalog=false // ordinary export, even with test env present
 result,err:=Generate(context.Background(),opts,fakeProbes())
 if !errors.Is(err,ErrCatalogRequired)||len(result.JSON)!=0{
  t.Fatalf("ordinary export accepted copied synthetic catalog (err=%v bytes=%d)",err,len(result.JSON))
 }
 t.Log("COPIED_CATALOG_ORDINARY_EXPORT=blocked: schema-valid, byte-identical external fixture")
 // Test mode is also intentionally limited to the pinned fixture by file identity.
 opts.AllowTestCatalog=true
 result,err=Generate(context.Background(),opts,fakeProbes())
 if err==nil||len(result.JSON)!=0{
  t.Fatal("test-only export accepted an external copied fixture")
 }
 t.Log("COPIED_CATALOG_TEST_MODE=blocked: only exact pinned CI fixture admitted")
}

func TestOverallStatusAcrossHardwareAndLLMFit(t *testing.T){
 opts:=testCatalog(t)
 suffix:="";if runtime.GOOS=="windows"{suffix=".exe"}
 mock:=filepath.Join(t.TempDir(),"llmfitmock"+suffix)
 build:=exec.Command("go","build","-o",mock,"../llmfit/testdata/mockcli")
 if b,err:=build.CombinedOutput();err!=nil{t.Fatalf("build synthetic CLI: %v; %s",err,b)}
 // No real llmfit download, network call or benchmark.
 failedHardware:=func()Probes{
  noData:=func(context.Context)(map[string]any,error){return nil,errors.New("synthetic hardware query failure")}
  return Probes{OS:noData,CPU:noData,RAM:noData,GPU:noData}
 }
 cases:=[]struct{
  name string
  probes Probes
  executable string
  mode string
  wantHardware string
  wantLLMFit string
  wantOverall string
  wantErrors int
 }{
  {"hardware_failed_llmfit_ok",failedHardware(),mock,"ok","failed","ok","partial",4},
  {"hardware_failed_llmfit_skipped",failedHardware(),"","","failed","skipped","failed",4},
  {"hardware_failed_llmfit_failed",failedHardware(),filepath.Join(t.TempDir(),"missing.exe"),"","failed","failed","failed",5},
  {"hardware_ok_llmfit_failed",fakeProbes(),filepath.Join(t.TempDir(),"missing.exe"),"","ok","failed","partial",1},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   t.Setenv("NEUROMETR_MOCK_MODE",tc.mode)
   selected:=opts
   selected.LLMFitExecutable=tc.executable
   selected.LLMFitTimeout=3*time.Second
   // testGenerate requires the full schema + semantic validator to PASS.
   result:=testGenerate(t,selected,tc.probes)
   report:=decodeReport(t,result.JSON)
   if result.HardwareStatus!=tc.wantHardware||result.LLMFitStatus!=tc.wantLLMFit||
     report["overall_status"]!=tc.wantOverall{
    t.Fatalf("status mismatch: hardware=%s llmfit=%s overall=%v; want %s/%s/%s",
     result.HardwareStatus,result.LLMFitStatus,report["overall_status"],
     tc.wantHardware,tc.wantLLMFit,tc.wantOverall)
   }
   components:=map[string]string{}
   for _,raw:=range report["components"].([]any){
    c:=raw.(map[string]any)
    components[c["component"].(string)]=c["status"].(string)
   }
   if components["hardware"]!=tc.wantHardware||components["llmfit"]!=tc.wantLLMFit||
      components["cloud"]!="skipped"{t.Fatalf("component status mismatch: %v",components)}
   if got:=len(report["errors"].([]any));got!=tc.wantErrors{
    t.Fatalf("want %d structured errors, got %d",tc.wantErrors,got)
   }
   if tc.wantLLMFit=="ok"{
    engines:=report["engines"].([]any)
    if len(engines)!=1||engines[0].(map[string]any)["status"]!="ok"{
      t.Fatal("successful llmfit engine result omitted")
    }
   }
   if report["catalog_ref"].(map[string]any)["origin"]!="cached"{
    t.Fatal("test-only catalog reference disappeared")
   }
   t.Logf("STATUS_REGRESSION=PASS hardware=%s llmfit=%s overall=%s full_validator=PASS",
    tc.wantHardware,tc.wantLLMFit,tc.wantOverall)
  })
 }
}
