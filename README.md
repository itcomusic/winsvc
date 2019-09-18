# winsvc
Provides creating and running Go Windows Service

### Features
- Restarts service on failure. Service will be restarted:  
  1. Threw panic
  2. Exit from run function had happened before context execution canceled (command of the stop was not sent) 
  3. Service had got command but it caught panic
- `context.Context` for graceful self shutdown
- Returns from `winsvc.Run` if it stops for a long time. `winsvc.TimeoutStop` is option which it default equals value 20s
- Package uses `os.Chdir` for easy using relative path

### Install
```go get -u github.com/itcomusic/winsvc```

### Example
```go
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/itcomusic/winsvc"
)

type Application struct {
	srv *http.Server
}

func main() {
	winsvc.Run(func(ctx context.Context) {
		app := New()
		if err := app.Run(ctx); err != nil {
			log.Printf("[ERROR] rest terminated with error, %s", err)
			return
		}
		log.Printf("[WARN] rest terminated")
	})
	// service has been just stopped, but process of the go has not stopped yet
	// that is why recommendation is to not write any logic
}

func New() *Application {
	mux := http.NewServeMux()
	server := &http.Server{
		Addr:    "0.0.0.0:8080",
		Handler: mux,
	}
	
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("hello winsvc"))
	})
	mux.HandleFunc("/exit", func(w http.ResponseWriter, r *http.Request) {
		// service will be restarted
		os.Exit(1)
	})
	mux.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		// service will be restarted
		server.Shutdown(context.TODO())
	})
    
	return &Application{srv: server}
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
$ sc.exe create "gowinsvc" binPath= "path\gowinsvc.exe" start= auto
$ sc.exe failure "gowinsvc" reset= 0 actions= restart/5000
```
