// +build windows

package winsvc

import (
	"flag"
	"io/ioutil"
	"os"
)

type Command int

const (
	CmdUnknown Command = iota
	CmdStart
	CmdStop
	CmdRestart
	CmdInstall
	CmdUninstall
	CmdRun
)

var (
	flagSvc = flag.NewFlagSet("winsvc", flag.ContinueOnError)
	action  *string

	// actionHandler is a list valid actions and functions to use in cmdHandler.
	actionHandler = map[string]struct {
		f   func() error
		cmd Command
	}{
		"start":     {Start, CmdStart},
		"stop":      {Stop, CmdStop},
		"restart":   {Restart, CmdRestart},
		"install":   {Install, CmdInstall},
		"uninstall": {Uninstall, CmdUninstall},
		"run":       {Run, CmdRun},
	}
)

func init() {
	flagSvc.SetOutput(ioutil.Discard)
	action = flagSvc.String("winsvc", "run", "Control the system service (install, start, restart, stop, uninstall)")
	flagSvc.Parse(os.Args[1:])
}

// cmdHandler returns function from a given action command string.
func cmdHandler() (func() error, Command, error) {
	h, ok := actionHandler[*action]
	if !ok {
		return nil, CmdUnknown, ErrCmd
	}
	return h.f, h.cmd, nil
}
