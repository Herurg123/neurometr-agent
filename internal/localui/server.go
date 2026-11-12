// Package localui provides an offline, single-process Go HTTP interface
// scoped to 127.0.0.1 and backed by the canonical diagnostic generator.
package localui

import (
 "context"
 "crypto/rand"
 "crypto/subtle"
 "embed"
 "encoding/hex"
 "encoding/json"
 "errors"
 "io"
 "net"
 "net/http"
 "strconv"
 "strings"
 "sync"
 "time"

 "github.com/Herurg123/neurometr-agent/internal/contract"
 "github.com/Herurg123/neurometr-agent/internal/diagnostic"
 "github.com/Herurg123/neurometr-agent/internal/history"
)

//go:embed ui/index.html ui/app.js ui/style.css
var assets embed.FS

const cookieName="neurometr_session"
const maxBody=1024

type GenerateFunc func(context.Context)(diagnostic.Result,error)
type Server struct{
 root string
 store *history.Store
 validate *contract.FullValidator
 generate GenerateFunc
 session string
 csrf string
 listener net.Listener
 server *http.Server
 busy chan struct{}
 closeOnce sync.Once
}
func randomSecret()(string,error){
 b:=make([]byte,32)
 if _,err:=rand.Read(b);err!=nil{return "",err}
 return hex.EncodeToString(b),nil
}
func New(root,historyDir string,generate GenerateFunc)(*Server,error){
 if root==""||historyDir==""||generate==nil{return nil,errors.New("UI configuration invalid")}
 v,err:=contract.NewFullValidator(root);if err!=nil{return nil,err}
 store,err:=history.New(historyDir,v);if err!=nil{return nil,err}
 session,err:=randomSecret();if err!=nil{return nil,err}
 csrf,err:=randomSecret();if err!=nil{return nil,err}
 s:=&Server{root:root,store:store,validate:v,generate:generate,session:session,csrf:csrf,busy:make(chan struct{},1)}
 return s,nil
}
func (s *Server) Start()(string,error){
 if s.listener!=nil{return "",errors.New("UI server already started")}
 ln,err:=net.Listen("tcp4","127.0.0.1:0")
 if err!=nil{return "",err}
 address:=ln.Addr().String()
 host,_,err:=net.SplitHostPort(address)
 if err!=nil||host!="127.0.0.1"{ln.Close();return "",errors.New("loopback listener invariant failed")}
 s.listener=ln
 s.server=&http.Server{
  Handler:http.HandlerFunc(s.serve),
  ReadHeaderTimeout:4*time.Second,
  ReadTimeout:10*time.Second,
  WriteTimeout:20*time.Second,
  IdleTimeout:20*time.Second,
  MaxHeaderBytes:8192,
 }
 go func(){_ = s.server.Serve(ln)}()
 return "http://"+address+"/",nil
}
func (s *Server) Close(ctx context.Context)error{
 if s.server==nil{return nil}
 return s.server.Shutdown(ctx)
}
func (s *Server) addr()string{
 if s.listener==nil{return ""}
 return s.listener.Addr().String()
}
func (s *Server) trustedRequest(r *http.Request)bool{
 if s.listener==nil||r.Host!=s.addr()||r.URL.IsAbs()||r.URL.RawQuery!=""{
  return false
 }
 origin:=r.Header.Get("Origin")
 if origin!=""&&origin!="http://"+s.addr(){return false}
 fetchSite:=r.Header.Get("Sec-Fetch-Site")
 if fetchSite!=""&&fetchSite!="same-origin"&&fetchSite!="none"{return false}
 if r.Header.Get("Sec-Fetch-Mode")=="navigate"&&r.Method!="GET"{return false}
 return true
}
func (s *Server) authenticated(r *http.Request)bool{
 cookie,err:=r.Cookie(cookieName)
 if err!=nil||len(cookie.Value)!=len(s.session){return false}
 return subtle.ConstantTimeCompare([]byte(cookie.Value),[]byte(s.session))==1
}
func (s *Server) csrfOK(r *http.Request)bool{
 if r.Header.Get("Origin")!="http://"+s.addr(){return false}
 token:=r.Header.Get("X-Neurometr-CSRF")
 if len(token)!=len(s.csrf){return false}
 return subtle.ConstantTimeCompare([]byte(token),[]byte(s.csrf))==1
}
func securityHeaders(w http.ResponseWriter){
 h:=w.Header()
 h.Set("Cache-Control","no-store, max-age=0")
 h.Set("X-Content-Type-Options","nosniff")
 h.Set("X-Frame-Options","DENY")
 h.Set("Referrer-Policy","no-referrer")
 h.Set("Cross-Origin-Resource-Policy","same-origin")
 h.Set("Cross-Origin-Opener-Policy","same-origin")
 h.Set("Permissions-Policy","camera=(), microphone=(), geolocation=(), payment=()")
 h.Set("Content-Security-Policy","default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'; object-src 'none'")
}
func jsonReply(w http.ResponseWriter,status int,v any){
 w.Header().Set("Content-Type","application/json; charset=utf-8")
 w.WriteHeader(status)
 _=json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter,status int){
 // Do not expose OS paths, raw logs, Go errors or tokens to HTTP clients.
 jsonReply(w,status,map[string]string{"error":http.StatusText(status)})
}
func (s *Server) serve(w http.ResponseWriter,r *http.Request){
 securityHeaders(w)
 if !s.trustedRequest(r){fail(w,http.StatusForbidden);return}
 if r.Method!="GET"&&r.Method!="POST"{w.Header().Set("Allow","GET, POST");fail(w,http.StatusMethodNotAllowed);return}
 if r.Method=="GET"{
  switch r.URL.Path{
  case "/":
   // Session cookie never appears in URL, scripts, JSON reports or logs.
   http.SetCookie(w,&http.Cookie{Name:cookieName,Value:s.session,Path:"/",HttpOnly:true,SameSite:http.SameSiteStrictMode})
   s.static(w,"ui/index.html","text/html; charset=utf-8")
   return
  case "/app.js":
   s.static(w,"ui/app.js","text/javascript; charset=utf-8");return
  case "/style.css":
   s.static(w,"ui/style.css","text/css; charset=utf-8");return
  }
 }
 if !strings.HasPrefix(r.URL.Path,"/api/"){fail(w,http.StatusNotFound);return}
 if !s.authenticated(r){fail(w,http.StatusUnauthorized);return}
 if r.Method=="GET"{
  switch {
  case r.URL.Path=="/api/session":
   jsonReply(w,http.StatusOK,map[string]string{"csrf":s.csrf})
  case r.URL.Path=="/api/history":
   entries,err:=s.store.List()
   if err!=nil{fail(w,http.StatusServiceUnavailable);return}
   jsonReply(w,http.StatusOK,entries)
  case strings.HasPrefix(r.URL.Path,"/api/report/"):
   s.open(w,strings.TrimPrefix(r.URL.Path,"/api/report/"),false)
  case strings.HasPrefix(r.URL.Path,"/api/export/"):
   s.open(w,strings.TrimPrefix(r.URL.Path,"/api/export/"),true)
  default:
   fail(w,http.StatusNotFound)
  }
  return
 }
 // No state-changing GET or HTML form submissions. Browser fetch must use
 // same-origin Origin, authenticated cookie and an independent CSRF header.
 if !s.csrfOK(r){fail(w,http.StatusForbidden);return}
 if r.URL.Path!="/api/run"{fail(w,http.StatusNotFound);return}
 if r.Header.Get("Content-Type")!="application/json"{fail(w,http.StatusUnsupportedMediaType);return}
 r.Body=http.MaxBytesReader(w,r.Body,maxBody)
 var request struct{}
 dec:=json.NewDecoder(r.Body)
 dec.DisallowUnknownFields()
 if err:=dec.Decode(&request);err!=nil{fail(w,http.StatusBadRequest);return}
 if dec.Decode(&request)!=io.EOF{fail(w,http.StatusBadRequest);return}
 select{
 case s.busy<-struct{}{}:
  defer func(){<-s.busy}()
 default:
  fail(w,http.StatusConflict)
  return
 }
 ctx,cancel:=context.WithTimeout(r.Context(),12*time.Second)
 defer cancel()
 output,err:=s.generate(ctx)
 if err!=nil{fail(w,http.StatusServiceUnavailable);return}
 // The existing generator validates before returning; validate once more at
 // the boundary, and history.Save validates independently before writing.
 if _,err=s.validate.ValidateBytes("diagnostic",output.JSON);err!=nil{
  fail(w,http.StatusServiceUnavailable);return
 }
 saved,err:=s.store.Save(output.JSON)
 if err!=nil{fail(w,http.StatusServiceUnavailable);return}
 jsonReply(w,http.StatusCreated,saved)
}
func (s *Server) static(w http.ResponseWriter,name,contentType string){
 data,err:=assets.ReadFile(name)
 if err!=nil{fail(w,http.StatusNotFound);return}
 w.Header().Set("Content-Type",contentType)
 w.Header().Set("Content-Length",strconv.Itoa(len(data)))
 _,_=w.Write(data)
}
func (s *Server) open(w http.ResponseWriter,id string,export bool){
 raw,_,err:=s.store.Load(id)
 if err!=nil{
  if errors.Is(err,history.ErrInvalidID){fail(w,http.StatusBadRequest)}else{fail(w,http.StatusNotFound)}
  return
 }
 w.Header().Set("Content-Type","application/json; charset=utf-8")
 if export{
  // ID is verified by history.Load with a strict UUIDv4 matcher.
  w.Header().Set("Content-Disposition",`attachment; filename="neurometr-`+id+`.json"`)
 }
 w.Header().Set("Content-Length",strconv.Itoa(len(raw)))
 _,_=w.Write(raw)
}
