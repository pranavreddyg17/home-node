package main

import (
	"context"
	"io"
	"time"
)

type guestServer interface{ Serve(io.ReadWriter) error }

// Closing the active channel interrupts its framed read on shutdown. Reconnect
// waits also observe cancellation; a channel opened during cancellation is closed.
func runGuest(ctx context.Context, agent guestServer, open func() (io.ReadWriteCloser, error)) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		stream, err := open()
		if ctx.Err() != nil {
			if stream != nil {
				_ = stream.Close()
			}
			return nil
		}
		if err == nil {
			stopped := make(chan struct{})
			go func() {
				select {
				case <-ctx.Done():
					stream.Close()
				case <-stopped:
				}
			}()
			_ = agent.Serve(stream)
			close(stopped)
			_ = stream.Close()
		}
		if ctx.Err() != nil {
			return nil
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
