# winsvc
Provides creating and running Go programs as windows service

### Features
- In package was set flag `winsvc` that control service (install, start, restart, stop, uninstall) and no need explicit realize flag and logic.
For using this flag, must run `winsvc.RunCmd()`, which will read `-winsvc` flag and execute specific command.
- Restarts service on failure. `winsvc.Config` has parameter `RestartOnFailure` which not must equal zero value for restarting.
- `context.CancelFunc` for graceful self shutdown.
- Kills process if it is stopping for a long time. `winsvc.Config` has parameter `TimeoutStop` which it default equals value setting in registry.
- Catchs panic and prints stack trace using default logger.
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
	winsvc.Servicer
	srv    *http.Server
	cancel context.CancelFunc
}

func main() {
	if err := New(); err != nil {
		log.Fatal(err)
	}
	if err := winsvc.RunCmd(); err != nil {
		log.Fatal(err)
	}
}

func New() error {
	mux := http.NewServeMux()

	app := &Application{srv: &http.Server{
		Addr:    "0.0.0.0:8080",
		Handler: mux,
	}}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	mux.HandleFunc("/fail", func(w http.ResponseWriter, r *http.Request) {
		// will be restarted
		os.Exit(1)
	})
	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		// graceful self shutdown
		app.cancel()
	})

	if err := winsvc.Init(app, winsvc.Config{
		Name:             "GoHTTPServer",
		DisplayName:      "Go HTTP server",
		Description:      "Go HTTP server example",
		RestartOnFailure: time.Second * 5, // restart service after failure
	}); err != nil {
		return err
	}
	return nil
}

func (s *Application) Start(cancel context.CancelFunc) {
	s.cancel = cancel
	err := s.srv.ListenAndServe()
	log.Printf("[WARN] http server terminated, %s", err)
}

func (s *Application) Stop() {
	defer log.Print("[WARN] shutdown rest server")
	ctx, _ := context.WithTimeout(context.Background(), time.Second*5)
	s.srv.Shutdown(ctx)
}
```
```sh
$ goservice.exe -winsvc install
$ goservice.exe -winsvc start
...
```