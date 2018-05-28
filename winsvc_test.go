// +build windows

package winsvc

import (
	"context"
	"os"
	"os/signal"
	"testing"
	"time"
)

type service struct {
	Servicer
	startFunc func(cancel context.CancelFunc)
	stopFunc  func()
}

func (s *service) Start(cancel context.CancelFunc) {
	s.startFunc(cancel)
}
func (s *service) Stop() {
	s.stopFunc()
}

var testsvc = &service{}
var _ = Init(testsvc, Config{
	Name: "ServiceTest",
})

func TestRunInterrupt(t *testing.T) {
	signalNotify = func(c chan<- os.Signal, sig ...os.Signal) {
		time.AfterFunc(time.Second*2, func() {
			c <- os.Interrupt
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	testsvc.startFunc = func(_ context.CancelFunc) {
		<-ctx.Done()
	}
	testsvc.stopFunc = func() {
		cancel()
	}

	go func() {
		select {
		case <-time.After(time.Second * 5):
			t.Fatal("service has not been stopped")
		case <-ctx.Done():
		}
	}()

	if err := Run(); err != nil {
		t.Fatal(err)
	}
}

func TestRunCancelFunc(t *testing.T) {
	signalNotify = signal.Notify
	ctx, cancel := context.WithCancel(context.Background())
	testsvc.startFunc = func(cancelInner context.CancelFunc) {
		defer cancelInner()
	}
	testsvc.stopFunc = func() {
		cancel()
	}

	go func() {
		select {
		case <-time.After(time.Second * 5):
			t.Fatal("service has not been stopped")
		case <-ctx.Done():
		}
	}()

	if err := Run(); err != nil {
		t.Fatal(err)
	}
}
