package llmfit

import (
 "context"
 "errors"
 "encoding/json"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

// TestLivePinnedCLI runs only on the pinned official Windows x64 binary
// provisioned on an ephemeral GitHub Actions runner. No model download or bench.
func TestLivePinnedCLI(t *testing.T) {
 exe:=os.Getenv("NEUROMETR_A2_REAL_EXE")
 if exe==""{t.Skip("real binary is provisioned only by cloud Windows CI")}
 ctx:=context.Background()
 version,err:=ProbeVersion(ctx,exe,10*time.Second)
 if err!=nil{t.Fatal(err)}
 t.Logf("LIVE_VERSION=%s",version)
 for _,op:=range []Operation{System,List,Fit,Recommend}{
  t.Run(string(op),func(t *testing.T){
   r,err:=Run(ctx,Request{Executable:exe,Operation:op,Timeout:75*time.Second})
   if err!=nil{
    var e *CLIError
    if op==List&&errors.As(err,&e)&&e.Kind==OutputTooLarge{
     t.Logf("LIVE_LIST_BOUNDED_REJECTION=confirmed STDOUT_CAP_MIB=8 CAPTURED_BYTES=%d EXIT=%d",r.StdoutBytes,r.ExitCode)
     return
    }
    t.Fatalf("real %s: %v; stderr=%s",op,err,sanitize(r.Stderr))
   }
   n:=0
   switch doc:=r.Document.(type){
   case []any:n=len(doc)
   case map[string]any:if a,ok:=doc["models"].([]any);ok{n=len(a)}
   }
   t.Logf("LIVE_OPERATION=%s EXIT=%d ELAPSED=%s MODELS=%d STDOUT_BYTES=%d STDERR_BYTES=%d",op,r.ExitCode,r.Duration,n,r.StdoutBytes,len(r.Stderr))
  })
 }
}

func sanitize(s []byte)string{
 // No whole raw logs in GitHub Actions. Known error fragments are enough for PoC.
 text:=strings.TrimSpace(string(s))
 if len(text)>300{text=text[:300]}
 return text
}

func customModel(name,params string,minRAM,recommendedRAM,minVRAM float64)map[string]any{
 return map[string]any{
  "name":name,"provider":"neurometr-poc",
  "parameter_count":params,
  "min_ram_gb":minRAM,"recommended_ram_gb":recommendedRAM,
  "min_vram_gb":minVRAM,
  "quantization":"Q4_K_M","context_length":4096,
  "use_case":"General chat",
 }
}
func writeCustom(t *testing.T,name string,large bool)string{
 t.Helper()
 models:=[]any{
  customModel("neurometr-poc/NeurometrA2ProbeA","1B",2,3,1),
  customModel("neurometr-poc/NeurometrA2ProbeB","5B",6,9,5),
 }
 if large{models[0]=customModel("neurometr-poc/NeurometrA2ProbeA","120B",120,150,100)}
 raw,err:=json.Marshal(models);if err!=nil{t.Fatal(err)}
 path:=filepath.Join(t.TempDir(),name+".json")
 if err=os.WriteFile(path,raw,0600);err!=nil{t.Fatal(err)}
 return path
}
func modelNames(doc any)[]string{
 var rows []any
 switch v:=doc.(type){
 case []any:rows=v
 case map[string]any:rows,_=v["models"].([]any)
 }
 names:=[]string{}
 for _,v:=range rows{if m,ok:=v.(map[string]any);ok{if n,ok:=m["name"].(string);ok{names=append(names,n)}}}
 return names
}
func objectModels(doc any)[]map[string]any{
 list:=[]map[string]any{}
 if d,ok:=doc.(map[string]any);ok{if a,ok:=d["models"].([]any);ok{for _,v:=range a{if m,ok:=v.(map[string]any);ok{list=append(list,m)}}}}
 return list
}
func visibleModel(doc any,name string)(map[string]any,bool){
 for _,m:=range objectModels(doc){if m["name"]==name{return m,true}}
 return nil,false
}
func brief(m map[string]any)string{
 if m==nil{return "<absent>"}
 var parts []string
 for _,key:=range []string{"fit_level","memory_required_gb","estimated_tps","score","parameter_count","run_mode"}{
  if v,ok:=m[key];ok{parts=append(parts,fmt.Sprintf("%s=%v",key,v))}
 }
 return strings.Join(parts,", ")
}

// This is an evidence-producing integration test, not an unconditional assertion
// that custom models affect real fit/ranking.
func TestLiveCustomModels(t *testing.T){
 exe:=os.Getenv("NEUROMETR_A2_REAL_EXE")
 if exe==""{t.Skip("real custom-model behavior requires official Windows CLI in cloud CI")}
 ctx:=context.Background()
 low:=writeCustom(t,"small",false)
 high:=writeCustom(t,"large",true)
 model:="neurometr-poc/NeurometrA2ProbeA"
 baseline,err:=Run(ctx,Request{Executable:exe,Operation:Search,Selector:"NeurometrA2ProbeA",Timeout:50*time.Second})
 if err!=nil{t.Logf("CUSTOM_BASELINE=not_tested err=%v",err)}else{t.Logf("CUSTOM_BASELINE_RESULTS=%d",len(modelNames(baseline.Document)))}
 check:=func(path string)(map[string]any,[]string,bool){
  found,err:=Run(ctx,Request{Executable:exe,Operation:Search,Selector:"NeurometrA2ProbeA",CustomModelsPath:path,Timeout:50*time.Second})
  if err!=nil{t.Logf("CUSTOM_SEARCH=failed err=%v stderr=%s",err,sanitize(found.Stderr));return nil,nil,false}
  detected:=false
  for _,name:=range modelNames(found.Document){if name==model{detected=true}}
  if !detected{t.Log("CUSTOM_LOAD=not_confirmed: model not present in search");return nil,nil,false}
  t.Log("CUSTOM_LOAD=confirmed: exact synthetic model visible in CLI search")
  info,err:=Run(ctx,Request{Executable:exe,Operation:Info,Selector:model,CustomModelsPath:path,Timeout:50*time.Second})
  if err!=nil{t.Logf("CUSTOM_INFO=not_confirmed err=%v stderr=%s",err,sanitize(info.Stderr))} 
  var detail map[string]any
  if err==nil{detail,_=visibleModel(info.Document,model)}
  ranked,fitErr:=Run(ctx,Request{Executable:exe,Operation:FitProvider,Selector:"neurometr-poc",CustomModelsPath:path,Timeout:60*time.Second})
  if fitErr!=nil{t.Logf("CUSTOM_FIT=not_confirmed err=%v stderr=%s",fitErr,sanitize(ranked.Stderr));return detail,nil,true}
  return detail,modelNames(ranked.Document),true
 }
 small,smallRanking,smallLoaded:=check(low)
 large,largeRanking,largeLoaded:=check(high)
 if !smallLoaded||!largeLoaded{t.Log("CUSTOM_EFFECT=not_confirmed: at least one CLI overlay load absent");return}
 t.Logf("CUSTOM_SMALL_FIT=%s",brief(small))
 t.Logf("CUSTOM_LARGE_FIT=%s",brief(large))
 if small!=nil&&large!=nil&&brief(small)!=brief(large){
  t.Log("CUSTOM_FIT_EFFECT=confirmed: real CLI fit/info fields differ between overlays")
 }else{t.Log("CUSTOM_FIT_EFFECT=not_confirmed: no comparable computed difference")}
 t.Logf("CUSTOM_SMALL_RANKING=%s",strings.Join(smallRanking,","))
 t.Logf("CUSTOM_LARGE_RANKING=%s",strings.Join(largeRanking,","))
 if len(smallRanking)>0&&len(largeRanking)>0&&strings.Join(smallRanking,",")!=strings.Join(largeRanking,","){
  t.Log("CUSTOM_RANKING_EFFECT=confirmed: real CLI provider-filtered ordering differs")
 }else{
  t.Log("CUSTOM_RANKING_EFFECT=not_confirmed: no observable ranking change under runner conditions")
 }
}
