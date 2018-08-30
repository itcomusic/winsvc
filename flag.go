// +build windows

package winsvc

import (
	"flag"
	"io/ioutil"
	"log"
	"os"
)

type command int

const (
	cmdHelp command = iota
	cmdStart
	cmdStop
	cmdRestart
	cmdInstall
	cmdUninstall
	cmdRun
)

// actionHandler is a list valid actions and functions to use in cmdHandler
var actionHandler = map[string]struct {
	f   func() error
	cmd command
}{
	"start":     {start, cmdStart},
	"stop":      {stop, cmdStop},
	"restart":   {restart, cmdRestart},
	"install":   {install, cmdInstall},
	"uninstall": {uninstall, cmdUninstall},
	"run":       {run, cmdRun},
}

type cmd struct {
	value   string
	typeCmd command
	handler func() error
}

func (c *cmd) Set(v string) error {
	if v == "" && !Interactive() {
		v = "run"
	}

	h, ok := actionHandler[v]
	if !ok {
		c.value = v
		return nil
	}
	c.value, c.typeCmd, c.handler = v, h.cmd, h.f

	return nil
}

func (c *cmd) String() string {
	return c.value
}

var (
	flagSvc = flag.NewFlagSet("winsvc", flag.ContinueOnError)
	action  = cmd{
		typeCmd: cmdHelp,
		handler: func() error {
			flagSvc.SetOutput(os.Stdout)
			flagSvc.PrintDefaults()
			return nil
		},
	}
)

func init() {
	flagSvc.SetOutput(ioutil.Discard)
	flagSvc.Var(&action, "winsvc", "Control the system service (install, start, restart, stop, uninstall)")
	flagSvc.Parse(os.Args[1:])
}

// runCmd executions command of the flag "winsvc".
func runCmd() error {
	switch action.typeCmd {
	case cmdInstall, cmdUninstall, cmdStart, cmdStop, cmdRestart, cmdHelp:
		if err := action.handler(); err != nil {
			log.Fatalf("winsvc: %s", err)
		}
		os.Exit(0)
	case cmdRun:
		return action.handler()
	}

	panic("unreachable code")
}
