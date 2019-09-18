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

type (
	option func(*manager)

	// runFunc is the function that can runOnce as windows service.
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
	runFunc func(ctx context.Context)
)

var (
	runOnce     sync.Once
	interactive = false
)

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

// Interactive returns false if running under the OS service manager and true otherwise.
func Interactive() bool {
	return interactive
}

// TimeoutStop is a option to specify timeout of stopping service.
// After expired timeout, process of service will be terminated.
// If is not set option, value will be equal default value 20s.
func TimeoutStop(t time.Duration) option {
	return func(m *manager) {
		m.timeout = t
	}
}

// DisablePanic is a option to disabling panic when exit from run function.
func DisablePanic() option {
	return func(m *manager) {
		m.disablePanic = true
	}
}

// signalNotify is a option to mock.
func signalNotify(f func(c chan<- os.Signal, sig ...os.Signal)) option {
	return func(m *manager) {
		m.signalNotify = f
	}
}

// start starts a service. Separated from sync.One for tests.
func start(r runFunc, opts ...option) {
	svcMan := &manager{
		svcHandler:   r,
		timeout:      time.Second * 20,
		signalNotify: signal.Notify,
	}

	for _, op := range opts {
		op(svcMan)
	}
	svcMan.run()
}

// Run initializes new windows service and runs command action.
// runFunc provides a place to initiate the service.
// runFunc function always has blocked and exit from it, means that service will be stopped correctly if is context was canceled.
// runFunc should not call os.Exit directly in the function, it is not correctly service stop.
// Context canceled it is mean that signal of stop got and need to stop run function.
func Run(r runFunc, opts ...option) {
	runOnce.Do(func() { start(r, opts...) })
}

type manager struct {
	svcHandler   runFunc
	ctxSvc       context.Context
	cancelSvc    context.CancelFunc
	svc.Handler  // svcHandler.Handler is controlled OS service manager
	timeout      time.Duration
	disablePanic bool
	signalNotify func(c chan<- os.Signal, sig ...os.Signal) // for mock and tests.
}

// run starts service.
func (m *manager) run() {
	m.ctxSvc, m.cancelSvc = context.WithCancel(context.Background())

	if !interactive {
		errRun := svc.Run("", m)
		if errRun != nil {
			panic(errRun)
		}
		return
	}
	finishRun := m.runFuncWithNotify()

	// waiting interrupt signal in interactive mode or cancel context
	sig := make(chan os.Signal, 1)
	m.signalNotify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
		m.cancelSvc()
	case <-finishRun:
		if !m.disablePanic {
			panic("exit from run function")
		}
		return
	}

	select {
	case <-finishRun:
	case <-time.After(m.timeout):
	}
}

// runFuncWithNotify returns context which will done when run function is stopped.
func (m *manager) runFuncWithNotify() <-chan struct{} {
	finishRun, cancelRun := context.WithCancel(context.Background())
	go func() {
		defer cancelRun()
		m.svcHandler(m.ctxSvc)
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
			if !m.disablePanic {
				panic("exit from run function")
			}
			return false, 1
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				m.cancelSvc() // cancel context svcHandler

				select {
				case <-finishRun:
				case <-time.After(m.timeout):
				}
				break loop
			}
		}
	}
	return false, 0
}
