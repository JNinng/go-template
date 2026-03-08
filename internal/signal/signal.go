package signal

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func WaitForShutdown(done func()) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	<-sigChan

	if done != nil {
		done()
	}
}

func ContextWithShutdown(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

		<-sigChan
		cancel()
	}()

	return ctx
}
