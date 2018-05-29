// +build windows

package winsvc

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"testing"
	"time"
)

type service struct {
	Servicer
	runFunc func(ctx context.Context) error
}

func (s *service) Run(ctx context.Context) error {
	return s.runFunc(ctx)
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

	ctxTest, cancelTest := context.WithCancel(context.Background())
	testsvc.runFunc = func(ctx context.Context) error {
		<-ctx.Done()
		cancelTest()
		return nil
	}

	go func() {
		select {
		case <-time.After(time.Second * 5):
			t.Fatal("service has not been stopped")
		case <-ctxTest.Done():
		}
	}()

	if err := Run(); err != nil {
		t.Fatal(err)
	}
}

func TestRunCancelFunc(t *testing.T) {
	signalNotify = signal.Notify
	testsvc.runFunc = func(_ context.Context) error { return nil }

	go func() {
		select {
		case <-time.After(time.Second * 5):
			t.Fatal("service has not been stopped")
		}
	}()

	if err := Run(); err != nil {
		t.Fatal(err)
	}
}

func TestReturnError(t *testing.T) {
	signalNotify = signal.Notify
	testsvc.runFunc = func(_ context.Context) error { return fmt.Errorf("test error") }

	if err := Run(); err != nil && err.Error() != "test error" {
		t.Fatal(err)
	}
}
