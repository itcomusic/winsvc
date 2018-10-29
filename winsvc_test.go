// +build windows

package winsvc

import (
	"context"
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

	Run(func(ctx context.Context) {
		<-ctx.Done()
		cancelTest()
	})
}

func TestInteractive_RunReturn(t *testing.T) {
	signalNotify = signal.Notify
	go func() {
		select {
		case <-time.After(time.Second * 5):
			t.Fatal("service has not been stopped")
		}
	}()

	Run(func(_ context.Context) { return })
}
