package localui

import (
 "bytes"
 "context"
 "encoding/json"
 "io"
 "net"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "sync/atomic"
 "testing"
 "time"

 "github.com/Herurg123/neurometr-agent/internal/diagnostic"
)

func testServer(t *testing.T,fn GenerateFunc)(*Server,string,[]byte){
 t.Helper()
 root,err:=filepath.Abs(filepath.Join("..",".."));if err!=nil{t.Fatal(err)}
 raw,err:=os.ReadFile(filepath.Join(root,"contracts","fixtures","F01_diagnostic_windows_ok.json"))
 if err!=nil{t.Fatal(err)}
 if fn==nil{fn=func(context.Context)(diagnostic.Result,error){return diagnostic.Result{JSON:raw},nil}}
 server,err:=New(root,filepath.Join(t.TempDir(),"history"),fn)
 if err!=nil{t.Fatal(err)}
 url,err:=server.Start();if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){ctx,c:=context.WithTimeout(context.Background(),3*time.Second);defer c();_ =server.Close(ctx)})
 return server,url,raw
}
func req(t *testing.T,client *http.Client,method,url string,cookie *http.Cookie,origin,csrf string,body []byte)*http.Response{
 t.Helper()
 var rbody io.Reader
 if body!=nil{rbody=bytes.NewReader(body)}
 r,err:=http.NewRequest(method,url,rbody);if err!=nil{t.Fatal(err)}
 if cookie!=nil{r.AddCookie(cookie)}
 if origin!=""{r.Header.Set("Origin",origin)}
 if csrf!=""{r.Header.Set("X-Neurometr-CSRF",csrf)}
 if method=="POST"{r.Header.Set("Content-Type","application/json")}
 resp,err:=client.Do(r);if err!=nil{t.Fatal(err)}
 return resp
}
func closed(resp *http.Response)[]byte{defer resp.Body.Close();b,_:=io.ReadAll(io.LimitReader(resp.Body,3*1024*1024));return b}
func tokens(t *testing.T,client *http.Client,url string)(*http.Cookie,string){
 t.Helper()
 first:=req(t,client,"GET",url,nil,"","",nil)
 page:=closed(first)
 if first.StatusCode!=200||!bytes.Contains(page,[]byte("Проверить компьютер")){t.Fatal("Russian UI not accessible")}
 if first.Header.Get("Content-Security-Policy")==""||first.Header.Get("Access-Control-Allow-Origin")!=""{t.Fatal("security headers or CORS")}
 var cookie *http.Cookie
 for _,c:=range first.Cookies(){if c.Name==cookieName{cookie=c}}
 if cookie==nil||!cookie.HttpOnly||cookie.SameSite!=http.SameSiteStrictMode||len(cookie.Value)!=64{
  t.Fatal("session cookie lacks HttpOnly, Strict or cryptographic entropy")
 }
 second:=req(t,client,"GET",url+"api/session",cookie,"","",nil)
 var obj map[string]string
 if err:=json.Unmarshal(closed(second),&obj);err!=nil{t.Fatal(err)}
 if second.StatusCode!=200||len(obj["csrf"])!=64{t.Fatal("CSRF token absent")}
 if strings.Contains(url,cookie.Value)||strings.Contains(url,obj["csrf"]){t.Fatal("secret leaked in URL")}
 return cookie,obj["csrf"]
}
func TestLoopbackSecurityAndHistoryAPI(t *testing.T){
 var invoked atomic.Int32
 root,err:=filepath.Abs(filepath.Join("..",".."));if err!=nil{t.Fatal(err)}
 report,err:=os.ReadFile(filepath.Join(root,"contracts","fixtures","F01_diagnostic_windows_ok.json"));if err!=nil{t.Fatal(err)}
 server,url,_:=testServer(t,func(context.Context)(diagnostic.Result,error){invoked.Add(1);return diagnostic.Result{JSON:report},nil})
 ip,port,err:=net.SplitHostPort(server.addr())
 if err!=nil||ip!="127.0.0.1"||port==""{t.Fatal("UI not bound exclusively to IPv4 loopback")}
 client:=&http.Client{Timeout:5*time.Second}
 cookie,csrf:=tokens(t,client,url)
 r:=req(t,client,"GET",url+"api/history",nil,"","",nil)
 if r.StatusCode!=401{t.Fatal("history exposed without session")};closed(r)
 r=req(t,client,"GET",url+"api/history",cookie,"http://attacker.invalid","","",nil)
 if r.StatusCode!=403{t.Fatal("cross-origin GET accepted")};closed(r)
 badHost,err:=http.NewRequest("GET",url+"api/history",nil);if err!=nil{t.Fatal(err)}
 badHost.Host="attacker.invalid"
 badHost.AddCookie(cookie)
 r,err=client.Do(badHost);if err!=nil{t.Fatal(err)}
 if r.StatusCode!=403{t.Fatal("DNS rebinding Host accepted")};closed(r)
 for _,tc:=range []struct{name string;cookie *http.Cookie;origin,csrf string;want int}{
  {"missing_session",nil,url[:len(url)-1],csrf,401},
  {"missing_origin",cookie,"",csrf,403},
  {"attacker_origin",cookie,"http://attacker.invalid",csrf,403},
  {"missing_csrf",cookie,url[:len(url)-1],"",403},
  {"wrong_csrf",cookie,url[:len(url)-1],strings.Repeat("a",64),403},
 }{
  t.Run(tc.name,func(t *testing.T){
   p:=req(t,client,"POST",url+"api/run",tc.cookie,tc.origin,tc.csrf,[]byte("{}"))
   if p.StatusCode!=tc.want{t.Fatalf("%s response %d want %d",tc.name,p.StatusCode,tc.want)};closed(p)
  })
 }
 options:=req(t,client,"OPTIONS",url+"api/run",cookie,url[:len(url)-1],csrf,nil)
 if options.StatusCode!=405||options.Header.Get("Access-Control-Allow-Origin")!=""{t.Fatal("CORS OPTIONS bypass")};closed(options)
 getRun:=req(t,client,"GET",url+"api/run",cookie,"","",nil)
 if getRun.StatusCode==200||getRun.StatusCode==201{t.Fatal("GET triggered state change")};closed(getRun)
 post:=req(t,client,"POST",url+"api/run",cookie,url[:len(url)-1],csrf,[]byte("{}"))
 if post.StatusCode!=201{t.Fatalf("authorized diagnostics failed: %d",post.StatusCode)}
 var entry struct{ID string `json:"id"`}
 if err:=json.Unmarshal(closed(post),&entry);err!=nil{t.Fatal(err)}
 if entry.ID==""||invoked.Load()!=1{t.Fatal("diagnostics invocation mismatch")}
 view:=req(t,client,"GET",url+"api/report/"+entry.ID,cookie,"","",nil)
 stored:=closed(view)
 if view.StatusCode!=200||!bytes.Equal(stored,report){t.Fatal("stored report differs from canonical full diagnostic")}
 if bytes.Contains(stored,[]byte(csrf))||bytes.Contains(stored,[]byte(cookie.Value)){t.Fatal("session secrets in diagnostic report")}
 export:=req(t,client,"GET",url+"api/export/"+entry.ID,cookie,"","",nil)
 if export.StatusCode!=200||!strings.Contains(export.Header.Get("Content-Disposition"),"attachment;")||!bytes.Equal(closed(export),report){
  t.Fatal("explicit full JSON export failed")
 }
 if invoked.Load()!=1{t.Fatal("GET caused an extra run")}
 listing:=req(t,client,"GET",url+"api/history",cookie,"","",nil)
 if listing.StatusCode!=200||!bytes.Contains(closed(listing),[]byte(entry.ID)){t.Fatal("local history missing saved report")}
 for _,bad:=range []string{"../etc/passwd","not-a-uuid","%2e%2e%2fsecret"}{
  view:=req(t,client,"GET",url+"api/report/"+bad,cookie,"","",nil)
  if view.StatusCode==200{t.Fatalf("path traversal accepted: %s",bad)};closed(view)
 }
 badBody:=req(t,client,"POST",url+"api/run",cookie,url[:len(url)-1],csrf,bytes.Repeat([]byte("a"),2048))
 if badBody.StatusCode<400{t.Fatal("oversized HTTP request accepted")};closed(badBody)
 if invoked.Load()!=1{t.Fatal("untrusted requests triggered generation")}
 t.Log("A3_4_LOOPBACK_HOST_ORIGIN_SESSION_CSRF=PASS CORS=DENIED GET_MUTATION=DENIED HISTORY_EXPORT=PASS")
}
func TestSingleConcurrentDiagnostic(t *testing.T){
 entered:=make(chan struct{},1)
 release:=make(chan struct{})
 var hits atomic.Int32
 root,err:=filepath.Abs(filepath.Join("..",".."));if err!=nil{t.Fatal(err)}
 report,err:=os.ReadFile(filepath.Join(root,"contracts","fixtures","F01_diagnostic_windows_ok.json"));if err!=nil{t.Fatal(err)}
 _,url,_:=testServer(t,func(ctx context.Context)(diagnostic.Result,error){
  hits.Add(1);entered<-struct{}{}
  select{case <-release:return diagnostic.Result{JSON:report},nil;case <-ctx.Done():return diagnostic.Result{},ctx.Err()}
 })
 client:=&http.Client{Timeout:5*time.Second}
 cookie,csrf:=tokens(t,client,url)
 finished:=make(chan int,1)
 go func(){r:=req(t,client,"POST",url+"api/run",cookie,url[:len(url)-1],csrf,[]byte("{}"));finished<-r.StatusCode;closed(r)}()
 select{case <-entered:case <-time.After(3*time.Second):t.Fatal("first diagnostic did not start")}
 second:=req(t,client,"POST",url+"api/run",cookie,url[:len(url)-1],csrf,[]byte("{}"))
 if second.StatusCode!=409{t.Fatalf("parallel start status %d, expected 409",second.StatusCode)};closed(second)
 close(release)
 select{case code:=<-finished:if code!=201{t.Fatalf("first run failed: %d",code)};case <-time.After(3*time.Second):t.Fatal("first run blocked")}
 if hits.Load()!=1{t.Fatal("multiple diagnostic processes started simultaneously")}
}
func TestNoSessionOnRawReports(t *testing.T){
 _,url,_:=testServer(t,nil)
 client:=&http.Client{Timeout:5*time.Second}
 root:=req(t,client,"GET",url,nil,"","",nil)
 page:=closed(root)
 if bytes.Contains(page,[]byte(cookieName+"=")){t.Fatal("cookie value embedded in HTML")}
 app:=req(t,client,"GET",url+"app.js",nil,"","",nil)
 js:=closed(app)
 if bytes.Contains(js,[]byte("innerHTML"))||bytes.Contains(js,[]byte("eval("))||bytes.Contains(js,[]byte("http://"))||
  bytes.Contains(js,[]byte("https://")){t.Fatal("UI script permits unsafe injection or remote resources")}
 if app.Header.Get("Access-Control-Allow-Origin")!=""{t.Fatal("static asset CORS enabled")}
}
