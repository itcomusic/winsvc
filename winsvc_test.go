// +build windows

package winsvc

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

func TestRun_Interrupt(t *testing.T) {
	ctxTest, cancelTest := context.WithCancel(context.Background())
	go func() {
		select {
		case <-time.After(time.Second * 5):
			t.Errorf("service has not been stopped")
		case <-ctxTest.Done():
		}
	}()

	start(func(ctx context.Context) {
		<-ctx.Done()
		cancelTest()
	}, signalNotify(func(c chan<- os.Signal, sig ...os.Signal) { c <- os.Interrupt }))
}

func TestRun_Panic(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("exp: error")
			return
		}

		exp := "exit from run function"
		if got := fmt.Sprintf("%v", r); got != exp {
			t.Errorf("exp: %s, got: %s", exp, got)
		}
	}()
	start(func(_ context.Context) {})
}

func TestRun_DisablePanic(t *testing.T) {
	runOnce = sync.Once{}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("exp: nil")
		}
	}()

	start(func(_ context.Context) {}, DisablePanic())
}
