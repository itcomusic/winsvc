# winsvc
Provides creating and running Go Windows Service

### Features
- `winsvc.Init` has parameter `cmd` which can be get using `winsvc.Flag(action string)` action may be equals install, start, run, restart, stop, uninstall.
`winsvc.Init(...)` reads `cmd` and execute specific command.
- Restarts service on failure. `winsvc.Config` has parameter `RestartOnFailure` which not must equal zero value for restarting. If exit from run function had happened before context execution canceled (command of the stop was not sent) service also will be restarted.
- `context.Context` for graceful self shutdown.
- Kills process if it is stopping for a long time. `winsvc.Config` has parameter `TimeoutStop` which it default equals value setting in registry.
- Package uses os.Chdir for easy using relative path.

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
	winsvcf := flag.String("winsvc", "unknown", "Control the system service (install, start, restart, stop, uninstall)")
	flag.Parse()
    
	err := winsvc.Init(winsvc.Config{
		Name:             "GoHTTPServer",
		DisplayName:      "Go HTTP server",
		Description:      "Go HTTP server example",
		RestartOnFailure: time.Second * 10, // restart service after failure
    	}, winsvc.Flag(*winsvcf),
	func(ctx context.Context) error {
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
```sh
$ goservice.exe -winsvc install
$ goservice.exe -winsvc start
...
```
