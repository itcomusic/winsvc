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

func TestInteractive_RunInterrupt(t *testing.T) {
	signalNotify = func(c chan<- os.Signal, sig ...os.Signal) {
		time.AfterFunc(time.Second*2, func() {
			c <- os.Interrupt
		})
	}

	ctxTest, cancelTest := context.WithCancel(context.Background())
	go func() {
		select {
		case <-time.After(time.Second * 5):
			t.Fatal("service has not been stopped")
		case <-ctxTest.Done():
		}
	}()

	err := Init(Config{Name: "test"},
		CmdRun,
		func(ctx context.Context) error {
			<-ctx.Done()
			cancelTest()
			return nil
		})

	if err != nil {
		t.Fatal(err)
	}
}

func TestInteractive_RunReturn(t *testing.T) {
	signalNotify = signal.Notify
	go func() {
		select {
		case <-time.After(time.Second * 5):
			t.Fatal("service has not been stopped")
		}
	}()

	err := Init(Config{Name: "test"}, CmdRun, func(_ context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
}

func TestInteractive_RunReturnError(t *testing.T) {
	signalNotify = signal.Notify
	err := Init(Config{Name: "test"}, CmdRun, func(_ context.Context) error { return fmt.Errorf("test error") })
	if err != nil && err.Error() != "test error" {
		t.Fatal(err)
	}
}
