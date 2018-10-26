# winsvc
Provides creating and running Go Windows Service

### Features
- Restarts service on failure. Service will be restarted:  
1.*Threw panic*  
2.*Exit from run function had happened before context execution canceled (command of the stop was not sent)*  
3.*Service had got command of the stop but it caught panic after*
- `context.Context` for graceful self shutdown
- Exit from winsvc.Run if it is stopping for a long time. `TimeoutStop` which it default equals value 20s
- Package uses os.Chdir for easy using relative path

### Install
```go get -u github.com/itcomusic/winsvc```

### Example
```go
type Application struct {
	srv *http.Server
}

func main() {
	err := winsvc.Run(func(ctx context.Context) error {
		app := New()

		return app.Run(ctx)
	})
	log.Printf("[WARN] rest terminated, %s", err)
}

func New() *Application {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("hello winsvc"))
	})
	mux.HandleFunc("/fail", func(w http.ResponseWriter, r *http.Request) {
		// will be restarted
		os.Exit(1)
	})

	return &Application{srv: &http.Server{
		Addr:    "0.0.0.0:8080",
		Handler: mux,
	}}
}

func (a *Application) Run(ctx context.Context) error {
	log.Print("[INFO] started rest")
	go func() {
		defer log.Print("[WARN] shutdown rest server")
		// shutdown on context cancellation
		<-ctx.Done()
		c, _ := context.WithTimeout(context.Background(), time.Second*5)
		a.srv.Shutdown(c)
	}()

	log.Printf("[INFO] started http server on port :%d", 8080)
	return a.srv.ListenAndServe()
}
```
### Using sc.exe
```sh
$ sc.exe create "gowinsvc" binPath= "...\winsvc.exe" start= auto
$ sc.exe failure "gowinsvc" reset= 0 actions= restart/5000
$ sc.exe description "gowinsvc" "description gowinsvc"
```
