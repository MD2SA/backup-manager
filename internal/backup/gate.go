package backup

import "context"

// Gate serializes operations that must not touch the target database at the
// same time (a backup dump and a restore apply, for example).
type Gate struct {
	ch chan struct{}
}

func NewGate() *Gate {
	return &Gate{ch: make(chan struct{}, 1)}
}

// Acquire blocks until the gate is free or ctx is cancelled.
func (g *Gate) Acquire(ctx context.Context) error {
	select {
	case g.ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release must only be called after a successful Acquire.
func (g *Gate) Release() {
	<-g.ch
}
