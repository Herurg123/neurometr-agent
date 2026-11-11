// llmfitmock is an A2 subprocess test helper, never a production dependency.
package main

import (
 "fmt"
 "os"
 "strings"
 "time"
)
func main(){
 mode:=os.Getenv("NEUROMETR_MOCK_MODE")
 args:=strings.Join(os.Args[1:]," ")
 if strings.Contains(args,"--version"){
  if mode=="bad_version"{fmt.Println("llmfit 9.9.9")}else{fmt.Println("llmfit 1.1.20")}
  return
 }
 switch mode{
 case "nonzero":
  fmt.Fprintln(os.Stderr,"synthetic nonzero failure")
  os.Exit(19)
 case "malformed":fmt.Println("{broken")
 case "trailing":fmt.Print(`{"system":{}}{"system":{}}`)
 case "wrong_shape":fmt.Println(`{"no_system":true}`)
 case "too_large":fmt.Print(strings.Repeat("x",9*1024*1024))
 case "sleep":time.Sleep(10*time.Second);fmt.Println(`{"system":{}}`)
 case "warning":
  fmt.Fprintln(os.Stderr,"synthetic informational warning")
  fmt.Println(`{"system":{"cpu":"mock"}}`)
 case "environment":
  token:=os.Getenv("GITHUB_TOKEN")!=""
  fmt.Println(fmt.Sprintf(`{"system":{"token_leaked":%t,"custom_path":%q}}`,token,os.Getenv("LLMFIT_CUSTOM_MODELS")))
 case "failed_json":
  fmt.Fprintln(os.Stderr,"synthetic error")
  fmt.Println(`{"error":"synthetic"}`)
  os.Exit(11)
 default:
  if strings.Contains(args,"list")||strings.Contains(args,"search"){fmt.Println(`[{"name":"synthetic/Model-1B"}]`);return}
  if strings.Contains(args,"info")||strings.Contains(args,"recommend")||strings.Contains(args,"fit"){fmt.Println(`{"system":{},"models":[{"name":"synthetic/Model-1B"}]}`);return}
  fmt.Println(`{"system":{"cpu":"mock"}}`)
 }
}
