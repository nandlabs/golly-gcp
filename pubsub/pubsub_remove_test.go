package pubsub

import (
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	"oss.nandlabs.io/golly/messaging"
)

// TestProvider_ImplementsListenerRemover is the load-bearing test: as long
// as Provider satisfies messaging.ListenerRemover, golly's manager-level
// dispatcher will route RemoveListeners / RemoveNamedListener through us.
func TestProvider_ImplementsListenerRemover(t *testing.T) {
	var _ messaging.ListenerRemover = (*Provider)(nil)
}

func TestProvider_RemoveListeners_IdempotentOnUnknownURL(t *testing.T) {
	p := &Provider{}
	u, _ := url.Parse("pubsub://no-such-sub")
	if err := p.RemoveListeners(u); err != nil {
		t.Errorf("RemoveListeners on unknown URL should return nil; got %v", err)
	}
	if err := p.RemoveNamedListener(u, "anything"); err != nil {
		t.Errorf("RemoveNamedListener on unknown URL should return nil; got %v", err)
	}
}

func TestProvider_RemoveListeners_CancelsTrackedEntries(t *testing.T) {
	p := &Provider{listeners: map[string][]pubsubListenerEntry{}}
	u, _ := url.Parse("pubsub://sub1")

	var cancels atomic.Int32
	mk := func(name string) pubsubListenerEntry {
		return pubsubListenerEntry{name: name, cancel: func() { cancels.Add(1) }}
	}
	p.listeners[u.Host] = []pubsubListenerEntry{mk(""), mk("worker"), mk("worker")}

	if err := p.RemoveListeners(u); err != nil {
		t.Fatalf("RemoveListeners: %v", err)
	}
	if cancels.Load() != 3 {
		t.Errorf("expected 3 cancel fns invoked; got %d", cancels.Load())
	}
	if _, ok := p.listeners[u.Host]; ok {
		t.Errorf("expected URL entry to be deleted from map")
	}
}

func TestProvider_RemoveNamedListener_KeepsOthers(t *testing.T) {
	p := &Provider{listeners: map[string][]pubsubListenerEntry{}}
	u, _ := url.Parse("pubsub://sub2")

	var unnamed, kept, dropped atomic.Int32
	mk := func(counter *atomic.Int32, name string) pubsubListenerEntry {
		return pubsubListenerEntry{name: name, cancel: func() { counter.Add(1) }}
	}
	p.listeners[u.Host] = []pubsubListenerEntry{
		mk(&unnamed, ""),
		mk(&kept, "alpha"),
		mk(&dropped, "beta"),
		mk(&dropped, "beta"),
	}

	if err := p.RemoveNamedListener(u, "beta"); err != nil {
		t.Fatalf("RemoveNamedListener: %v", err)
	}
	if dropped.Load() != 2 {
		t.Errorf("expected 2 'beta' listeners cancelled; got %d", dropped.Load())
	}
	if unnamed.Load() != 0 || kept.Load() != 0 {
		t.Errorf("other listeners should not have been cancelled; unnamed=%d kept=%d",
			unnamed.Load(), kept.Load())
	}
	if len(p.listeners[u.Host]) != 2 {
		t.Fatalf("expected 2 listeners remaining; got %d", len(p.listeners[u.Host]))
	}
}

func TestProvider_RemoveNamedListener_LastEntryDeletesURL(t *testing.T) {
	p := &Provider{listeners: map[string][]pubsubListenerEntry{}}
	u, _ := url.Parse("pubsub://sub3")
	p.listeners[u.Host] = []pubsubListenerEntry{{name: "solo", cancel: func() {}}}
	if err := p.RemoveNamedListener(u, "solo"); err != nil {
		t.Fatalf("RemoveNamedListener: %v", err)
	}
	if _, ok := p.listeners[u.Host]; ok {
		t.Errorf("URL entry should be deleted when last listener is removed")
	}
}

func TestProvider_RemoveListeners_ConcurrentSafe(t *testing.T) {
	p := &Provider{listeners: map[string][]pubsubListenerEntry{}}
	u, _ := url.Parse("pubsub://race")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			p.mu.Lock()
			p.listeners[u.Host] = append(p.listeners[u.Host], pubsubListenerEntry{cancel: func() {}})
			p.mu.Unlock()
		}()
		go func() {
			defer wg.Done()
			_ = p.RemoveListeners(u)
		}()
	}
	wg.Wait()
	// No assertion needed beyond -race not firing.
}
