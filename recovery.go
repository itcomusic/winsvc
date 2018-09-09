// +build windows

package winsvc

import (
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

// https://msdn.microsoft.com/en-us/library/windows/desktop/ms681988(v=vs.85).aspx
const (
	serviceConfigFailureActionsFlag = 4
)

// https://msdn.microsoft.com/en-us/library/windows/desktop/ms685937(v=vs.85).aspx
type serviceFailureActionsFlag struct {
	failureActionsOnNonCrashFailures int32
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

	if err := s.SetRecoveryActions([]mgr.RecoveryAction{mgr.RecoveryAction{
		Type:  mgr.ServiceRestart,
		Delay: m.RestartOnFailure,
	}}, 5); err != nil {
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
