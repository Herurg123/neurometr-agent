// Package diagnostic implements the bounded A3.2 hardware-first report pipeline.
// It does not implement a localhost server, network probes, remote catalog updates
// or inference/model benchmarking.
package diagnostic

import (
 "context"
 "crypto/rand"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "unicode"
 "strings"
 "time"

 "github.com/Herurg123/neurometr-agent/internal/contract"
 "github.com/Herurg123/neurometr-agent/internal/llmfit"
)

const ProducerVersion = "0.1.0"
const ProbeBudget = 1500 * time.Millisecond
const FastBudget = 8 * time.Second

var ErrCatalogRequired = errors.New("A3.2: no approved production catalog/trust policy; ordinary diagnostic export is disabled")
var ErrNotWindows = errors.New("native Windows x64 diagnostics only")

type Probe func(context.Context) (map[string]any,error)

// Probes enables isolated native collectors in production and deterministic
// synthetic/failure collectors in Go tests. Each receives its own deadline.
type Probes struct { OS, CPU, RAM, GPU Probe }

type Options struct {
 ContractRoot string
 CatalogPath string
 // AllowTestCatalog is restricted to cloud CI fixture evidence, never a
 // production configuration or assertion of a trusted catalog.
 AllowTestCatalog bool
 HardwareTimeout time.Duration
 PerProbeTimeout time.Duration
 LLMFitExecutable string
 LLMFitSHA256 string
 LLMFitTimeout time.Duration
}

type Result struct {
 JSON []byte
 HardwareElapsed time.Duration
 TotalElapsed time.Duration
 HardwareStatus string
 LLMFitStatus string
}

// Strict no-PII report encoding uses schemas' approved fields only.
// Every failure message is static, never raw system stderr, paths or host metadata.
func evidence(kind, source string) map[string]any {
 return map[string]any{"kind":kind,"source":source}
}
// scrubModelName drops untrusted device labels that could disclose user
// identity, host paths, IPs or other information not required by A3.2.
func scrubModelName(v any)any{
 name,ok:=v.(string)
 if !ok{return nil}
 name=strings.TrimSpace(name)
 if name==""||len(name)>180{return nil}
 low:=strings.ToLower(name)
 for _,secret:=range []string{"/users/","/home/","password","token","secret","://",".local"}{
  if strings.Contains(low,secret){return nil}
 }
 if strings.ContainsRune(name,rune(92)){return nil}
 if strings.ContainsRune(name,'@')&&!strings.Contains(name," @ "){return nil}
 for _,token:=range strings.FieldsFunc(name,func(r rune)bool{return !unicode.IsDigit(r)&&r!='.'}){
  parts:=strings.Split(token,".")
  if len(parts)==4{
   looksLikeIPv4:=true
   for _,part:=range parts{
    if part==""||len(part)>3{looksLikeIPv4=false;break}
    for _,digit:=range part{if digit<'0'||digit>'9'{looksLikeIPv4=false}}
   }
   if looksLikeIPv4{return nil}
  }
 }
 for _,ch:=range name{if unicode.IsControl(ch)||ch==rune(0x2028)||ch==rune(0x2029){return nil}}
 return name
}
func scrubHardware(h map[string]any){
 if cpu,ok:=h["cpu"].(map[string]any);ok{cpu["model"]=scrubModelName(cpu["model"])}
 if osValue,ok:=h["os"].(map[string]any);ok{
  if version,ok:=osValue["version"].(string);ok{
   if len(version)>32||strings.ContainsAny(version,string([]rune{32,10,13,9,92,47})){osValue["version"]=nil}
  }
 }
 if gpus,ok:=h["gpus"].([]any);ok{
  for _,row:=range gpus{if g,ok:=row.(map[string]any);ok{g["name"]=scrubModelName(g["name"])}}
 }
}
func unknownOS()map[string]any{
 return map[string]any{"family":"windows","version":nil,"arch":"x64","evidence":evidence("unknown","agent:windows-api:unavailable")}
}
func unknownCPU()map[string]any{
 return map[string]any{"model":nil,"physical_cores":nil,"logical_processors":nil,"evidence":evidence("unknown","agent:windows-api:unavailable")}
}
func unknownRAM()map[string]any{
 return map[string]any{"physical_total_bytes":nil,"available_bytes":nil,"available_observed_at":nil,"evidence":evidence("unknown","agent:windows-api:unavailable")}
}

func runProbe(ctx context.Context, budget time.Duration, fn Probe) (map[string]any,error){
 if fn==nil{return nil,errors.New("probe unavailable")}
 if budget<=0||budget>ProbeBudget{budget=ProbeBudget}
 step,cancel:=context.WithTimeout(ctx,budget)
 defer cancel()
 type response struct{ value map[string]any;err error }
 out:=make(chan response,1)
 go func(){
  // Native calls cannot always be interrupted by a context cancellation.
  // A bounded caller does not wait for an unresponsive API; no WMI fallback.
  v,err:=fn(step)
  select{case out<-response{v,err}:default:}
 }()
 select{
 case x:=<-out:return x.value,x.err
 case <-step.Done():return nil,step.Err()
 }
}

type hardware struct {
 data map[string]any
 status string
 errs []any
 limitations []any
}
func collect(ctx context.Context,probes Probes,perProbe time.Duration) hardware{
 values:=map[string]any{"os":unknownOS(),"cpu":unknownCPU(),"ram":unknownRAM(),"gpus":[]any{},"npus":[]any{}}
 succeeded:=0
 h:=hardware{data:values,status:"ok",limitations:[]any{
  "NPU discovery not implemented; empty npus list does not prove NPU absence.",
  "DXGI shared system memory is an allocation ceiling, not additional physical RAM; do not add it to RAM.",
  "DXGI adapter data does not measure free VRAM or successful GPU runtime/backend execution.",
 }}
 rows:=[]struct{name,code string;fn Probe}{
  {"os","OS_QUERY_FAILED",probes.OS},
  {"cpu","CPU_QUERY_FAILED",probes.CPU},
  {"ram","RAM_QUERY_FAILED",probes.RAM},
  {"gpus","GPU_QUERY_FAILED",probes.GPU},
 }
 for _,r:=range rows{
  v,err:=runProbe(ctx,perProbe,r.fn)
  if err!=nil{
   h.status="partial"
   code:=r.code
   if errors.Is(err,context.DeadlineExceeded)||errors.Is(err,context.Canceled){code="HARDWARE_PROBE_TIMEOUT"}
   h.errs=append(h.errs,map[string]any{"component":"hardware","code":code,"message":"A bounded Windows hardware probe did not complete.","retryable":true})
   h.limitations=append(h.limitations,fmt.Sprintf("%s not fully detected; unavailable fields remain unknown.",r.name))
   continue
  }
  if v!=nil{
   succeeded++
   if r.name=="gpus"{
    // Return {"adapters": [...]} from the native DXGI collector.
    if list,ok:=v["adapters"].([]any);ok{values["gpus"]=list}
   }else{values[r.name]=v}
  }
 }
 if succeeded==0{h.status="failed"}
 if len(h.errs)>0&&h.status=="ok"{h.status="partial"}
 scrubHardware(values)
 return h
}

// A3.2 intentionally has no production catalog trust anchor. Mere structural
// validity, local origin or a computed SHA-256 never establishes publisher trust.
// Only the canonical pinned fixture is admitted in explicitly marked CI test mode.
// A later, separately approved trust policy must replace this fail-closed gate.
func catalogRef(v *contract.FullValidator,root string,opts Options)(map[string]any,error){
 if !opts.AllowTestCatalog{return nil,ErrCatalogRequired}
 if os.Getenv("NEUROMETR_A3_TEST_ONLY")!="1"{
  return nil,errors.New("test-only catalog mode is not enabled")
 }
 if opts.CatalogPath=="" {return nil,ErrCatalogRequired}
 if !filepath.IsAbs(opts.CatalogPath){return nil,errors.New("test catalog path must be absolute")}
 expectedPath:=filepath.Join(root,"contracts","fixtures","catalog_valid.json")
 expectedInfo,err:=os.Stat(expectedPath)
 if err!=nil{return nil,errors.New("pinned CI catalog fixture unavailable")}
 st,err:=os.Lstat(opts.CatalogPath)
 if err!=nil{return nil,errors.New("test catalog unavailable")}
 if !st.Mode().IsRegular()||st.Size()>contract.CatalogLimit||!os.SameFile(st,expectedInfo){
  return nil,errors.New("test mode accepts only the exact pinned CI catalog fixture")
 }
 b,err:=os.ReadFile(opts.CatalogPath)
 if err!=nil{return nil,errors.New("pinned test catalog could not be read")}
 // NewFullValidator verified the SHA-256 lock for the pinned fixture above.
 obj,err:=v.ValidateBytes("catalog",b)
 if err!=nil{return nil,errors.New("pinned test catalog failed full v0.1.0 validation")}
 digest:=sha256.Sum256(b)
 return map[string]any{
  "version":obj["catalog_version"],
  "sha256":hex.EncodeToString(digest[:]),
  "origin":"cached",
  // Test-only: schema compatibility does not prove producer authenticity.
  "compatibility":"passed",
 },nil
}
func uuidV4()(string,error){
 var b [16]byte
 if _,err:=rand.Read(b[:]);err!=nil{return "",errors.New("secure random run ID unavailable")}
 b[6]=(b[6]&0x0f)|0x40
 b[8]=(b[8]&0x3f)|0x80
 return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",b[0:4],b[4:6],b[6:8],b[8:10],b[10:16]),nil
}
func utc(t time.Time)string{return t.UTC().Format(time.RFC3339Nano)}

// Generate returns JSON only after a successful full schema and semantic check.
// Hardware does NOT require the llmfit binary or any network connectivity.
// Approved catalog provenance is required by canonical v0.1.0 catalog_ref.
// As A3.2 has no trusted production catalog, normal export fails closed;
// only the exact pinned fixture is accepted in explicit test-only mode.
func Generate(ctx context.Context,opts Options,probes Probes)(Result,error){
 started:=time.Now().UTC()
 result:=Result{}
 if opts.ContractRoot==""{return result,errors.New("contract root is required")}
 if opts.AllowTestCatalog&&os.Getenv("NEUROMETR_A3_TEST_ONLY")!="1"{
  return result,errors.New("test catalog authorization requires isolated CI test environment")
 }
 v,err:=contract.NewFullValidator(opts.ContractRoot);if err!=nil{return result,err}
 catalog,err:=catalogRef(v,opts.ContractRoot,opts)
 if err!=nil{return result,err}
 id,err:=uuidV4();if err!=nil{return result,err}
 if opts.HardwareTimeout<=0||opts.HardwareTimeout>FastBudget{opts.HardwareTimeout=FastBudget}
 hCtx,cancel:=context.WithTimeout(ctx,opts.HardwareTimeout)
 hardwareStart:=time.Now()
 h:=collect(hCtx,probes,opts.PerProbeTimeout)
 result.HardwareElapsed=time.Since(hardwareStart)
 cancel()
 result.HardwareStatus=h.status
 components:=[]any{map[string]any{"component":"hardware","status":h.status}}
 errorsList:=append([]any{},h.errs...)
 engines:=[]any{}
 llmfitStatus:="skipped"
 limitations:=h.limitations
 limitations=append(limitations,"TEST ONLY: pinned synthetic catalog was schema-validated, not authenticated as a production catalog.")
 
 if opts.LLMFitExecutable!=""{
  timeBudget:=opts.LLMFitTimeout
  if timeBudget<=0||timeBudget>30*time.Second{timeBudget=30*time.Second}
  r,runErr:=llmfit.Run(ctx,llmfit.Request{
   Executable:opts.LLMFitExecutable,ExpectedExecutableSHA256:opts.LLMFitSHA256,
   Operation:llmfit.System,Timeout:timeBudget,
  })
  if runErr==nil{
   llmfitStatus="ok"
   engines=append(engines,map[string]any{"name":"llmfit","version":llmfit.Version,"status":"ok","sha256":nil})
   limitations=append(limitations,"llmfit CLI system JSON returned; no inference runtime, model load or measured TPS was tested.")
  }else{
   llmfitStatus="failed"
   code:="LLMFIT_UNAVAILABLE"
   retry:=true
   var ce *llmfit.CLIError
   if errors.As(runErr,&ce){
    switch ce.Kind{
    case llmfit.Timeout:code="LLMFIT_TIMEOUT"
    case llmfit.Missing:code="LLMFIT_MISSING"
    case llmfit.ExitNonzero:code="LLMFIT_EXIT_NONZERO"
    case llmfit.InvalidJSON:code="LLMFIT_INVALID_JSON"
    case llmfit.UnexpectedShape:code="LLMFIT_BAD_OUTPUT"
    case llmfit.VersionMismatch:code="LLMFIT_VERSION_MISMATCH";retry=false
    case llmfit.OutputTooLarge:code="LLMFIT_OUTPUT_TOO_LARGE"
    case llmfit.InvalidBinary, llmfit.InvalidRequest:code="LLMFIT_INVALID_BINARY";retry=false
    }
   }
   errorsList=append(errorsList,map[string]any{"component":"llmfit","code":code,"message":"Optional llmfit CLI check failed; hardware observations were preserved.","retryable":retry})
   var version any=nil
   if r.Version!=""{version=llmfit.Version}
   engines=append(engines,map[string]any{"name":"llmfit","version":version,"status":"failed","sha256":nil})
  }
 }else{
  limitations=append(limitations,"Optional llmfit CLI not requested: engine, runtime execution, model fit and throughput not tested.")
 }
 components=append(components,map[string]any{"component":"llmfit","status":llmfitStatus})
 components=append(components,map[string]any{"component":"cloud","status":"skipped"})
 result.LLMFitStatus=llmfitStatus
 // STATUS_EXECUTION_INVARIANTS.md: partial needs usable hardware or
 // llmfit work and a degraded component. Failed means no usable component.
 overall:="ok"
 degraded:=h.status=="partial"||h.status=="failed"||llmfitStatus=="failed"
 usable:=h.status=="ok"||h.status=="partial"||llmfitStatus=="ok"
 if !usable {
  overall="failed"
 }else if degraded{
  overall="partial"
 }
 report:=map[string]any{
  "kind":"neurometr.diagnostic",
  "schema_version":"0.1.0",
  "run_id":id,
  "started_at":utc(started),
  "finished_at":utc(time.Now()),
  "overall_status":overall,
  "producer":map[string]any{"name":"neurometr-agent","version":ProducerVersion,"platform":"windows-x64"},
  "engines":engines,
  "catalog_ref":catalog,
  "hardware":h.data,
  "runtime_checks":[]any{},
  "model_assessments":[]any{},
  "cloud_checks":[]any{},
  "components":components,
  "errors":errorsList,
  "limitations":limitations,
 }
 raw,err:=json.Marshal(report);if err!=nil{return result,errors.New("report encoding failed")}
 if len(raw)>contract.DiagnosticLimit{return result,errors.New("diagnostic exceeds 2 MiB")}
 if _,err=v.ValidateBytes("diagnostic",raw);err!=nil{
  return result,fmt.Errorf("generated diagnostic failed full contract validation: %w",err)
 }
 result.JSON=raw
 result.TotalElapsed=time.Since(started)
 return result,nil
}
