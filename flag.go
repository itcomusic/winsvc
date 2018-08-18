// +build windows

package winsvc

import (
	"flag"
	"io/ioutil"
	"os"

	)

type command int

const (
	cmdUnknown command = iota
	cmdStart
	cmdStop
	cmdRestart
	cmdInstall
	cmdUninstall
	cmdRun
)

var (
	flagSvc = flag.NewFlagSet("winsvc", flag.ContinueOnError)
	action  *string

	// actionHandler is a list valid actions and functions to use in cmdHandler.
	actionHandler = map[string]struct {
		f   func() error
		cmd command
	}{
		"start":     {start, cmdStart},
		"stop":      {stop, cmdStop},
		"restart":   {restart, cmdRestart},
		"install":   {install, cmdInstall},
		"uninstall": {uninstall, cmdUninstall},
		"run":       {run, cmdRun},
		// todo: -h
	}
)

func init() {
	flagSvc.SetOutput(ioutil.Discard)
	action = flagSvc.String("winsvc", "", "Control the system service (install, start, restart, stop, uninstall)")
	flagSvc.Parse(os.Args[1:])
}

// cmdHandler returns function from a given action command string.
func cmdHandler() (func() error, command, error) {
	h, ok := actionHandler[*action]
	if !ok {
		return nil, cmdUnknown, errCmd
	}

	return h.f, h.cmd, nil
}
