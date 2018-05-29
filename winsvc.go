// Package winsvc provides create and run programs as windows service.

// +build windows

package winsvc

import (
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/itcomusic/winsvc/internal/svc/mgr"

	"context"

	"syscall"

	"log"

	"sync"

	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
)

var (
	// ErrEmptyName is an error returned by invalid config of service name.
	ErrEmptyName = errors.New("name field is required")
	// ErrSvcInit is an error returned by action with not initialized service.
	ErrSvcInit = errors.New("service was not initialized")
	// ErrExist is an error returned by try install existing service.
	ErrExist = errors.New("service has already existed")
	// ErrNotExist is an error returned by try uninstall not existent service.
	ErrNotExist = errors.New("service was not installed")
	// ErrCmd is an error returned by unknown value command "winsvc".
	ErrCmd = errors.New("unknown action")
)

var (
	// variable signal.Notify function for mock and tests.
	signalNotify    = signal.Notify
	interactive     = false
	timeStopDefault time.Duration
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
	timeStopDefault = getStopTimeout()
}

// getStopTimeout fetches the time before windows will kill the service, but it is not working.
func getStopTimeout() time.Duration {
	defaultTimeout := time.Millisecond * 20000
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control`, registry.READ)
	if err != nil {
		return defaultTimeout
	}
	sv, _, err := key.GetStringValue("WaitToKillServiceTimeout")
	if err != nil {
		return defaultTimeout
	}
	v, err := strconv.Atoi(sv)
	if err != nil {
		return defaultTimeout
	}
	return time.Millisecond * time.Duration(v)
}

// An Config represents information a configuration for a service.
type Config struct {
	// Name is required name of the service. No spaces suggested.
	Name string
	// DisplayName is a optional field to display name. Spaces allowed.
	DisplayName string
	// Description is a optional field to long description of service.
	Description string
	// Username is a optional field to run service by username.
	Username string
	// Password is a optional field to password of a username.
	Password string
	// Arguments is a optional field to arguments of a launching.
	Arguments []string
	// RestartOnFailure is a optional field to specify time in milliseconds of delay for restart service after failed exit
	// (os.Exit(0), os.Exit(1) and etc.). If is not set option, service will not be restarted after failed.
	RestartOnFailure time.Duration
	// TimeoutStop is a field to specify timeout of stopping service in milliseconds.
	// After expired timeout, process of service will be terminated.
	// If is not set option, default value will be equal value setting in registry.
	TimeoutStop time.Duration
	// Executable is a optional field to specify the executable for service.
	// If it is empty the current executable is used.
	Executable string
	// Dependencies is a optional field to set dependencies of the service.
	Dependencies []string
}

func (c Config) execPath() (string, error) {
	if len(c.Executable) != 0 {
		return filepath.Abs(c.Executable)
	}
	return os.Executable()
}

// Servicer is the interface implemented by types that can start as windows service.
//
//   1. OS service manager executes user program.
//   2. User program sees it is executed from a service manager (when IsInteractive is false).
//   3. User program calls winsvc.Run() which is blocked.
//   4. Servicer.Run() is called.
//   5. User program runs.
//   6. OS service manager signals the user program to stop.
//   7. Context was canceled.
//   8. winsvc.Run returns.
//   9. User program should quickly exit.
type Servicer interface {
	// Run provides a place to initiate the service.
	// Run function always has blocked and exit from it, means that service will be stopped correctly.
	// Run should not call os.Exit directly in the function, it is not correctly service stop and service will be
	// restarted if "RestartOnFailure" option is enabled.
	// Context canceled it is mean that signal of stop got and need to stop Run function.
	Run(ctx context.Context) error
}

var svcMan *manager

type errorSvc struct {
	sync.RWMutex
	err error
}

type manager struct {
	Config
	svc       Servicer
	ctxSvc    context.Context
	cancelSvc context.CancelFunc
	errRun    *errorSvc
	// svc.Handler is controlled OS service manager
	svc.Handler
}

// Init initializes new windows service based on a Servicer and configuration.
func Init(service Servicer, c Config) error {
	if len(c.Name) == 0 {
		return ErrEmptyName
	}
	if c.TimeoutStop == 0 {
		c.TimeoutStop = timeStopDefault
	}
	svcMan = &manager{
		Config: c,
		svc:    service,
		errRun: &errorSvc{
			sync.RWMutex{},
			nil,
		},
	}
	return nil
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

// RunCmd executions command of the flag "winsvc".
func RunCmd() error {
	handler, err := cmdHandler()
	if err != nil {
		return err
	}
	return handler()
}

// Run starts service and should be called shortly after the program entry point.
// After finished running, function Run will stop blocking.
// After stops blocking, the program must exit shortly after.
func Run() error {
	if svcMan == nil {
		return ErrSvcInit
	}
	svcMan.ctxSvc, svcMan.cancelSvc = context.WithCancel(context.Background())

	if !interactive {
		errRun := svc.Run(svcMan.Name, svcMan)
		if errSvc := svcMan.getError(); errSvc != nil {
			return errSvc
		}
		return errRun
	}

	finishRun := svcMan.runFuncWithNotify()

	// waiting interrupt signal in interactive mode or cancel context
	sig := make(chan os.Signal, 2)
	signalNotify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
		svcMan.cancelSvc()
	case <-finishRun:
	}

	select {
	case <-finishRun:
	case <-time.After(svcMan.TimeoutStop):
	}
	return svcMan.getError()
}

// runFuncWithNotify returns context which will done when run function is stopped.
func (m *manager) runFuncWithNotify() <-chan struct{} {
	finishRun, cancelRun := context.WithCancel(context.Background())
	go func() {
		defer cancelRun()
		defer m.recoverer()
		m.setError(m.svc.Run(m.ctxSvc))
	}()
	return finishRun.Done()
}

// Install creates new service and setups up the given service in the OS service manager.
// This may require greater rights. Will return an error if it is already installed.
func Install() error {
	if svcMan == nil {
		return ErrSvcInit
	}
	if err := svcMan.install(); err != nil {
		return err
	}

	if svcMan.RestartOnFailure != 0 {
		if err := svcMan.setRestartOnFailure(); err != nil {
			return err
		}
	}
	return nil
}

func (m *manager) install() error {
	path, err := m.execPath()
	if err != nil {
		return err
	}
	mg, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer mg.Disconnect()
	s, err := mg.OpenService(m.Name)
	if err == nil {
		s.Close()
		return ErrExist
	}

	s, err = mg.CreateService(m.Name, path, mgr.Config{
		DisplayName:      m.DisplayName,
		Description:      m.Description,
		StartType:        mgr.StartAutomatic,
		ServiceStartName: m.Username,
		Password:         m.Password,
		Dependencies:     m.Dependencies,
	}, m.Arguments...)
	if err != nil {
		return err
	}
	defer s.Close()
	return nil
}

// Uninstall removes the given service from the OS service manager.
// This may require greater rights. Will return an error if the service is not present.
func Uninstall() error {
	if svcMan == nil {
		return ErrSvcInit
	}
	mg, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer mg.Disconnect()
	s, err := mg.OpenService(svcMan.Name)
	if err != nil {
		return ErrNotExist
	}
	defer s.Close()
	err = s.Delete()
	if err != nil {
		return err
	}
	return nil
}

// Start signals to the OS service manager to start service.
func Start() error {
	if svcMan == nil {
		return ErrSvcInit
	}
	mg, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer mg.Disconnect()

	s, err := mg.OpenService(svcMan.Name)
	if err != nil {
		return err
	}
	defer s.Close()
	return s.Start()
}

// Stop signals to the OS service manager to stop service and waits stop service.
func Stop() error {
	if svcMan == nil {
		return ErrSvcInit
	}
	mg, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer mg.Disconnect()

	s, err := mg.OpenService(svcMan.Name)
	if err != nil {
		return err
	}
	defer s.Close()

	return svcMan.stopWithWait(s)
}

func (m *manager) stopWithWait(s *mgr.Service) error {
	// First Stop the service. Then wait for the service to
	// actually stop before starting it.
	status, err := s.Control(svc.Stop)
	if err != nil {
		return err
	}
	timeDuration := time.Millisecond * 50
	timeout := time.After(timeStopDefault + (timeDuration * 2))
	tick := time.NewTicker(timeDuration)
	defer tick.Stop()

loop:
	for status.State != svc.Stopped {
		select {
		case <-tick.C:
			status, err = s.Query()
			if err != nil {
				return err
			}
		case <-timeout:
			break loop
		}
	}
	return nil
}

// Restart restarts service. Restart signals to the OS service manager the given service should stop then start.
func Restart() error {
	if svcMan == nil {
		return ErrSvcInit
	}
	mg, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer mg.Disconnect()

	s, err := mg.OpenService(svcMan.Name)
	if err != nil {
		return err
	}
	defer s.Close()

	err = svcMan.stopWithWait(s)
	if err != nil {
		return err
	}

	return s.Start()
}

// recoverer recovers panic and prints error and stack trace in log.
// With the option "RestartOnFailure" enabled: if panic happened in run function, service will not be restarted.
func (m *manager) recoverer() {
	if rvr := recover(); rvr != nil {
		log.Printf("panic: %s\n\n%s", rvr, debug.Stack())
		select {
		case <-m.ctxSvc.Done():
		default:
			os.Exit(2)
		}
	}
}

// Execute manages status of the service.
func (m *manager) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const cmdAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	finishRun := m.runFuncWithNotify()

	changes <- svc.Status{State: svc.Running, Accepts: cmdAccepted}
	forceStop := finishRun
loop:
	for {
		select {
		case <-forceStop:
			forceStop = nil // one-shot
			m.sendCmdStop()
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				m.cancelSvc() // cancel context svc
				select {
				case <-finishRun:
				case <-time.After(m.TimeoutStop):
				}
				break loop
			}
		}
	}
	return false, 0
}

// sendCmdStop sends command to OS service manager to stop service.
func (m *manager) sendCmdStop() error {
	mg, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer mg.Disconnect()

	s, err := mg.OpenService(m.Name)
	if err != nil {
		return err
	}
	defer s.Close()
	_, err = s.Control(svc.Stop)
	return err
}
