// +build windows

package winsvc

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

// https://msdn.microsoft.com/en-us/library/windows/desktop/ms681988(v=vs.85).aspx
const (
	serviceConfigFailureActions     = 2
	serviceConfigFailureActionsFlag = 4
)

const (
	scActionNone = iota
	scActionRestart
	scActionReboot
	scActionRunCommand
)

// https://msdn.microsoft.com/en-us/library/windows/desktop/ms685937(v=vs.85).aspx
type serviceFailureActionsFlag struct {
	failureActionsOnNonCrashFailures int32
}

// https://msdn.microsoft.com/en-us/library/windows/desktop/ms685939(v=vs.85).aspx
type serviceFailureActions struct {
	dwResetPeriod uint32
	lpRebootMsg   *uint16
	lpCommand     *uint16
	cActions      uint32
	scAction      *serviceAction
}

// https://msdn.microsoft.com/en-us/library/windows/desktop/ms685126(v=vs.85).aspx
type serviceAction struct {
	actionType uint16
	delay      uint32
}

func (m *manager) setRestartOnFailure() error {
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

	action := serviceAction{
		actionType: scActionRestart,
		delay:      uint32(time.Duration(m.RestartOnFailure).Seconds() * 1e3),
	}
	failActions := serviceFailureActions{
		dwResetPeriod: 5,
		lpRebootMsg:   nil,
		lpCommand:     nil,
		cActions:      1,
		scAction:      &action,
	}

	if err := windows.ChangeServiceConfig2(s.Handle, serviceConfigFailureActions, (*byte)(unsafe.Pointer(&failActions))); err != nil {
		return err
	}
	flag := serviceFailureActionsFlag{
		failureActionsOnNonCrashFailures: 1,
	}
	if err := windows.ChangeServiceConfig2(s.Handle, serviceConfigFailureActionsFlag, (*byte)(unsafe.Pointer(&flag))); err != nil {
		return err
	}
	return nil
}
