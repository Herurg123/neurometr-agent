// localui launches the A3.4 offline, single-process Windows localhost browser UI.
// It never fetches models, runs benchmarks or performs network cloud checks.
package main

import (
 "context"
 "flag"
 "fmt"
 "os"
 "os/signal"
 "path/filepath"
 "runtime"
 "syscall"
 "time"

 "github.com/Herurg123/neurometr-agent/internal/diagnostic"
 "github.com/Herurg123/neurometr-agent/internal/history"
 "github.com/Herurg123/neurometr-agent/internal/localui"
)
func main(){
 rootArg:=flag.String("root","","Agent directory containing canonical contracts and approved bundled catalog")
 noBrowser:=flag.Bool("no-browser",false,"do not auto-open a browser (CI smoke only)")
 flag.Parse()
 if runtime.GOOS!="windows"||runtime.GOARCH!="amd64"{
  fmt.Fprintln(os.Stderr,"Локальный интерфейс A3.4 поддерживает только Windows x64.")
  os.Exit(2)
 }
 root:=*rootArg
 if root==""{
  executable,err:=os.Executable()
  if err!=nil{fmt.Fprintln(os.Stderr,"Не удалось определить каталог приложения.");os.Exit(2)}
  root=filepath.Dir(executable)
 }
 absRoot,err:=filepath.Abs(root)
 if err!=nil{fmt.Fprintln(os.Stderr,"Некорректный каталог приложения.");os.Exit(2)}
 dir,err:=history.DefaultDir()
 if err!=nil{fmt.Fprintln(os.Stderr,"Каталог личной истории недоступен.");os.Exit(2)}
 generate:=func(ctx context.Context)(diagnostic.Result,error){
  // No optional llmfit, cloud checks or user-supplied catalog in the fast path.
  return diagnostic.Generate(ctx,diagnostic.Options{ContractRoot:absRoot},diagnostic.NativeProbes())
 }
 ui,err:=localui.New(absRoot,dir,generate)
 if err!=nil{
  fmt.Fprintln(os.Stderr,"Локальная история, контракт или защита ACL недоступны. Запуск отменён.")
  os.Exit(2)
 }
 url,err:=ui.Start()
 if err!=nil{fmt.Fprintln(os.Stderr,"Не удалось открыть защищённый loopback-порт.");os.Exit(2)}
 // No session or CSRF value is printed or appended to the URL.
 fmt.Println("NEUROMETR_LOCAL_UI="+url)
 fmt.Println("Интерфейс работает только на этом компьютере. Для остановки закройте программу.")
 if !*noBrowser{_ = openBrowser(url)}
 ctx,stop:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM)
 defer stop()
 <-ctx.Done()
 deadline,cancel:=context.WithTimeout(context.Background(),5*time.Second)
 defer cancel()
 _=ui.Close(deadline)
}
