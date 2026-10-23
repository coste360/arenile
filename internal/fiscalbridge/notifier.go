package fiscalbridge

import "sync"

// notifier wakes the long-polls of an establishment (observer). Each
// establishment has a generation channel that is closed and replaced on
// broadcast, so any number of waiters is woken without per-waiter
// bookkeeping and a broadcast with no waiter costs nothing.
type notifier struct {
	mu  sync.Mutex
	gen map[string]chan struct{}
}

func newNotifier() *notifier { return &notifier{gen: map[string]chan struct{}{}} }

// subscribe returns the channel closed by the next broadcast for key.
func (n *notifier) subscribe(key string) <-chan struct{} {
	n.mu.Lock()
	defer n.mu.Unlock()
	ch, ok := n.gen[key]
	if !ok {
		ch = make(chan struct{})
		n.gen[key] = ch
	}
	return ch
}

// broadcast wakes the current subscribers of key.
func (n *notifier) broadcast(key string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if ch, ok := n.gen[key]; ok {
		close(ch)
		delete(n.gen, key)
	}
}

// broadcastAll wakes every subscriber.
func (n *notifier) broadcastAll() {
	n.mu.Lock()
	defer n.mu.Unlock()
	for k, ch := range n.gen {
		close(ch)
		delete(n.gen, k)
	}
}
