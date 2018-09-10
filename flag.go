// +build windows

package winsvc

import (
	"log"
	"os"
)

type command int

const (
	cmdUnknown command = iota
	CmdStart
	CmdStop
	CmdRestart
	CmdInstall
	CmdUninstall
	CmdRun
)

func Flag(action string) command {
	switch action {
	case "start":
		return CmdStart
	case "stop":
		return CmdStop
	case "restart":
		return CmdRestart
	case "install":
		return CmdInstall
	case "uninstall":
		return CmdUninstall
	case "run":
		return CmdRun
	default:
		return cmdUnknown
	}
}

// cmdHandler is a list valid actions and functions to use in cmdHandler
var cmdHandler = map[command]func() error{
	cmdUnknown:   func() error { return errCmd },
	CmdStart:     start,
	CmdStop:      stop,
	CmdRestart:   restart,
	CmdInstall:   install,
	CmdUninstall: uninstall,
	CmdRun:       run,
}

// runCmd executions command of the flag "winsvc".
func runCmd(cmd command) error {
	handler := cmdHandler[cmd]

	switch cmd {
	case cmdUnknown, CmdInstall, CmdUninstall, CmdStart, CmdStop, CmdRestart:
		if err := handler(); err != nil {
			log.Fatalf("winsvc: %s", err)
		}

		os.Exit(0)
	case CmdRun:
		return handler()
	}
	panic("unreachable code")
}
