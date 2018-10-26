// Package winsvc provides create and run programs as windows service.

// +build windows

package winsvc

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows/svc"
)

var (
	srErr error
	start sync.Once
)

var (
	// variable signal.Notify function for mock and tests.
	signalNotify = signal.Notify
	interactive  = false
	// TimeoutStop is a field to specify timeout of stopping service in milliseconds.
	// After expired timeout, process of service will be terminated.
	// If is not set option, value will be equal default value 10000 .
	TimeoutStop = time.Millisecond * 10000
)

// Interactive returns false if running under the OS service manager and true otherwise.
func Interactive() bool {
	return interactive
}

func init() {
	ex, errEx := os.Executable()
	if errEx != nil {
		panic(errEx)
	}

	if err := os.Chdir(filepath.Dir(ex)); err != nil {
		panic(err)
	}

	var errIn error
	interactive, errIn = svc.IsAnInteractiveSession()
	if errIn != nil {
		panic(errIn)
	}
}

// runFunc is the function that can start as windows service.
//
//   1. OS service manager executes user program.
//   2. User program sees it is executed from a service manager (when Interactive() is false).
//   3. User program calls winsvc.Run(...) which is blocked.
//   4. runFunc is called.
//   5. User program runs.
//   6. OS service manager signals the user program to stop.
//   7. Context was canceled.
//   8. winsvc.Run returns.
//   9. User program should quickly exit.
type runFunc func(ctx context.Context) error

type errorSvc struct {
	sync.RWMutex
	err error
}

type manager struct {
	svcHandler runFunc
	ctxSvc     context.Context
	cancelSvc  context.CancelFunc
	errRun     *errorSvc
	// svcHandler.Handler is controlled OS service manager
	svc.Handler
}

// Run initializes new windows service and runs command action.
// runFunc provides a place to initiate the service.
// runFunc function always has blocked and exit from it, means that service will be stopped correctly if is context was canceled.
// runFunc should not call os.Exit directly in the function, it is not correctly service stop and service will be
// restarted if "RestartOnFailure" option is enabled.
// Context canceled it is mean that signal of stop got and need to stop run function.
func Run(r runFunc) error {
	start.Do(func() {
		svcMan := &manager{
			svcHandler: r,
			errRun: &errorSvc{
				sync.RWMutex{},
				nil,
			},
		}
		srErr = svcMan.run()
	})

	return srErr
}

func (m *manager) setError(err error) {
	m.errRun.Lock()
	defer m.errRun.Unlock()
	m.errRun.err = err
}

func (m *manager) getError() error {
	m.errRun.RLock()
	defer m.errRun.RUnlock()
	return m.errRun.err
}

// run starts service.
func (m *manager) run() error {
	m.ctxSvc, m.cancelSvc = context.WithCancel(context.Background())

	if !interactive {
		errRun := svc.Run("", m)
		if errSvc := m.getError(); errSvc != nil {
			return errSvc
		}
		m.setError(errRun)
		return errRun
	}
	finishRun := m.runFuncWithNotify()

	// waiting interrupt signal in interactive mode or cancel context
	sig := make(chan os.Signal, 1)
	signalNotify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
		m.cancelSvc()
	case <-finishRun:
	}

	select {
	case <-finishRun:
	case <-time.After(TimeoutStop):
	}
	return m.getError()
}

// runFuncWithNotify returns context which will done when run function is stopped.
func (m *manager) runFuncWithNotify() <-chan struct{} {
	finishRun, cancelRun := context.WithCancel(context.Background())
	go func() {
		defer cancelRun()
		m.setError(m.svcHandler(m.ctxSvc))
	}()
	return finishRun.Done()
}

// Execute manages status of the service.
func (m *manager) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const cmdAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	finishRun := m.runFuncWithNotify()

	changes <- svc.Status{State: svc.Running, Accepts: cmdAccepted}
loop:
	for {
		select {
		case <-finishRun:
			panic("unexpected exit from run function")
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				m.cancelSvc() // cancel context svcHandler

				select {
				case <-finishRun:
				case <-time.After(TimeoutStop):
				}
				break loop
			}
		}
	}
	return false, 0
}
