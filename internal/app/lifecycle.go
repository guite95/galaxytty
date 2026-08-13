package app

import (
	"context"
	"fmt"
	"github.com/galaxytty/galaxytty/internal/domain"
	"sync"
)

type State string

const (
	Disconnected State = "disconnected"
	Connecting   State = "connecting"
	Ready        State = "ready"
	Reconnecting State = "reconnecting"
	ShuttingDown State = "shutting_down"
	Failed       State = "error"
)

type Lifecycle struct {
	mu       sync.RWMutex
	state    State
	once     sync.Once
	cancel   context.CancelFunc
	display  domain.VirtualDisplayManager
	notifier domain.Notifier
	err      error
}

func NewLifecycle(d domain.VirtualDisplayManager, n domain.Notifier) *Lifecycle {
	return &Lifecycle{state: Disconnected, display: d, notifier: n}
}
func (l *Lifecycle) State() State { l.mu.RLock(); defer l.mu.RUnlock(); return l.state }
func (l *Lifecycle) Transition(to State) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	valid := map[State]map[State]bool{Disconnected: {Connecting: true, ShuttingDown: true}, Connecting: {Ready: true, Failed: true, Disconnected: true}, Ready: {Reconnecting: true, ShuttingDown: true, Failed: true}, Reconnecting: {Ready: true, Disconnected: true, Failed: true, ShuttingDown: true}, Failed: {Reconnecting: true, ShuttingDown: true}}
	if !valid[l.state][to] {
		return fmt.Errorf("invalid lifecycle transition %s -> %s", l.state, to)
	}
	l.state = to
	return nil
}
func (l *Lifecycle) BindCancel(c context.CancelFunc) { l.cancel = c }
func (l *Lifecycle) Shutdown(ctx context.Context) error {
	l.once.Do(func() {
		l.mu.Lock()
		l.state = ShuttingDown
		l.mu.Unlock()
		if l.cancel != nil {
			l.cancel()
		}
		if l.notifier != nil {
			l.err = l.notifier.Close(ctx)
		}
		if l.display != nil {
			if e := l.display.Stop(ctx); l.err == nil {
				l.err = e
			}
		}
		l.mu.Lock()
		l.state = Disconnected
		l.mu.Unlock()
	})
	return l.err
}
