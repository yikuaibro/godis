package tcp

/**
 * A tcp server
 */

import (
	"context"
	"fmt"
	"github.com/hdt3213/godis/src/interface/tcp"
	"github.com/hdt3213/godis/src/lib/logger"
	"github.com/hdt3213/godis/src/lib/sync/atomic"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Config struct {
	Address    string        `yaml:"address"`
	MaxConnect uint32        `yaml:"max-connect"`
	Timeout    time.Duration `yaml:"timeout"`
}

func ListenAndServeWithSignal(cfg *Config, handler tcp.Handler) error {
	closeChan := make(chan struct{})
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigCh
		switch sig {
		case syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT:
			closeChan <- struct{}{}
		}
	}()
	return ListenAndServe(cfg, handler, closeChan)
}

func ListenAndServe(cfg *Config, handler tcp.Handler, closeChan <-chan struct{}) error {
	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return fmt.Errorf("listen err: %v", err)
	}

	// listen signal
	var closing atomic.AtomicBool
	go func() {
		<-closeChan
		logger.Info("shutting down...")
		closing.Set(true)
		_ = listener.Close() // listener.Accept() will return err immediately
		_ = handler.Close()  // close connections
	}()

	// listen port
	logger.Info(fmt.Sprintf("bind: %s, start listening...", cfg.Address))
	defer func() {
		// close during unexpected error
		_ = listener.Close()
		_ = handler.Close()
	}()
	ctx := context.Background()
	var waitDone sync.WaitGroup
	for {
		conn, err := listener.Accept()
		if err != nil {
			if closing.Get() {
				logger.Info("waiting disconnect...")
				waitDone.Wait()
				return nil // handler will be closed by defer
			}
			logger.Error(fmt.Sprintf("accept err: %v", err))
			continue
		}
		// handle
		logger.Info("accept link")
		waitDone.Add(1)
		go func() {
			defer func() {
				waitDone.Done()
			}()
			handler.Handle(ctx, conn)
		}()
	}
}
