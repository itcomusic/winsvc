// Package winsvc provides create and run programs as windows service.

// +build windows

package winsvc

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

var (
	// errCmd is an error return by unknown command of the winsvc.
	errCmd = errors.New("unknown command")
	// errEmptyName is an error returned by invalid config of service name.
	errEmptyName = errors.New("name field is required")
	// errSvcInit is an error returned by action with not initialized service.
	errSvcInit = errors.New("service was not initialized")
	// errExist is an error returned by try install existing service.
	errExist = errors.New("service has already existed")
	// errNotExist is an error returned by try uninstall not existent service.
	errNotExist = errors.New("service was not installed")
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

// getStopTimeout fetches the time before process will be finished.
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

// runFunc is the function that can start as windows service.
//
//   1. OS service manager executes user program.
//   2. User program sees it is executed from a service manager (when Interactive() is false).
//   3. User program calls winsvc.Init(...) which is blocked.
//   4. runFunc is called.
//   5. User program runs.
//   6. OS service manager signals the user program to stop.
//   7. Context was canceled.
//   8. winsvc.Init returns.
//   9. User program should quickly exit.
type runFunc func(ctx context.Context) error

var svcMan *manager

type errorSvc struct {
	sync.RWMutex
	err error
}

type manager struct {
	Config
	svcHandler runFunc
	ctxSvc     context.Context
	cancelSvc  context.CancelFunc
	errRun     *errorSvc
	// svcHandler.Handler is controlled OS service manager
	svc.Handler
}

// Init initializes new windows service and runs command action.
// runFunc provides a place to initiate the service.
// runFunc function always has blocked and exit from it, means that service will be stopped correctly if is context was canceled.
// runFunc should not call os.Exit directly in the function, it is not correctly service stop and service will be
// restarted if "RestartOnFailure" option is enabled.
// Context canceled it is mean that signal of stop got and need to stop run function.
func Init(c Config, cmd command, run runFunc) error {
	if !Interactive() {
		cmd = CmdRun
	}

	if len(c.Name) == 0 {
		log.Fatalf("winsvc: %s", errEmptyName)
	}

	if c.TimeoutStop == 0 {
		c.TimeoutStop = timeStopDefault
	}

	svcMan = &manager{
		Config:     c,
		svcHandler: run,
		errRun: &errorSvc{
			sync.RWMutex{},
			nil,
		},
	}

	return runCmd(cmd)
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
// After finished running, function run will stop blocking.
// After stops blocking, the program must exit shortly after.
func run() error {
	if svcMan == nil {
		return errSvcInit
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
	sig := make(chan os.Signal, 1)
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
		m.setError(m.svcHandler(m.ctxSvc))
	}()
	return finishRun.Done()
}

// install creates new service and setups up the given service in the OS service manager.
// This may require greater rights. Will return an error if it is already installed.
func install() error {
	if svcMan == nil {
		return errSvcInit
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
		return errExist
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

// uninstall removes the given service from the OS service manager.
// This may require greater rights. Will return an error if the service is not present.
func uninstall() error {
	if svcMan == nil {
		return errSvcInit
	}

	mg, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer mg.Disconnect()

	s, err := mg.OpenService(svcMan.Name)
	if err != nil {
		return errNotExist
	}
	defer s.Close()

	err = s.Delete()
	if err != nil {
		return err
	}

	return nil
}

// start signals to the OS service manager to start service.
func start() error {
	if svcMan == nil {
		return errSvcInit
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

// stop signals to the OS service manager to stop service and waits stop service.
func stop() error {
	if svcMan == nil {
		return errSvcInit
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
	// first stop the service. Then wait for the service to actually stop before starting it
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

// restart restarts service. Restart signals to the OS service manager the given service should stop then start.
func restart() error {
	if svcMan == nil {
		return errSvcInit
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
loop:
	for {
		select {
		// unexpected exit from run function
		case <-finishRun:
			return true, 1
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}

				m.cancelSvc() // cancel context svcHandler
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
