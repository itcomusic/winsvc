// +build windows

package winsvc

import (
	"flag"
	"io/ioutil"
	"os"
)

var (
	flagSvc = flag.NewFlagSet("winsvc", flag.ContinueOnError)
	action  *string
	// actionHandler is a list valid actions and functions to use in cmdHandler.
	actionHandler = map[string]func() error{
		"start":     Start,
		"stop":      Stop,
		"restart":   Restart,
		"install":   Install,
		"uninstall": Uninstall,
		"run":       Run,
	}
)

func init() {
	flagSvc.SetOutput(ioutil.Discard)
	action = flagSvc.String("winsvc", "run", "Control the system service (install, start, restart, stop, uninstall)")
	flagSvc.Parse(os.Args[1:])
}

// cmdHandler returns function from a given action command string.
func cmdHandler() (func() error, error) {
	h, ok := actionHandler[*action]
	if !ok {
		return nil, ErrCmd
	}
	return h, nil
}
