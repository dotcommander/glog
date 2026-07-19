package sse

import (
	"sync"
	"testing"
	"time"
)

func TestRemoveClientAfterCloseIsIdempotent(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	client := make(chan Event, 1)

	hub.AddClient(client)
	hub.Close()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RemoveClient panicked after Close: %v", r)
		}
	}()

	hub.RemoveClient(client)
}

func TestBroadcastDoesNotBlockClientLifecycle(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	slowClient := make(chan Event, 1)
	slowClient <- Event{Type: "original"}
	hub.AddClient(slowClient)

	fanoutReached := make(chan struct{})
	releaseFanout := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseFanout) }) })
	hub.warnSlowClients = func(string, int) {
		close(fanoutReached)
		<-releaseFanout
	}

	broadcastDone := make(chan struct{})
	go func() {
		hub.Broadcast(Event{Type: "new"})
		close(broadcastDone)
	}()
	<-fanoutReached

	newClient := make(chan Event, 1)
	addDone := make(chan struct{})
	go func() {
		hub.AddClient(newClient)
		close(addDone)
	}()
	waitForCompletion(t, "AddClient", addDone)

	removeDone := make(chan struct{})
	go func() {
		hub.RemoveClient(newClient)
		close(removeDone)
	}()
	waitForCompletion(t, "RemoveClient", removeDone)

	releaseOnce.Do(func() { close(releaseFanout) })
	waitForCompletion(t, "Broadcast", broadcastDone)

	if got := <-slowClient; got.Type != "original" {
		t.Fatalf("buffered event = %q; want original", got.Type)
	}
}

func TestBroadcastDeliversToReadyClientsAndCountsDrops(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	readyClient := make(chan Event, 1)
	slowClientOne := make(chan Event, 1)
	slowClientTwo := make(chan Event, 1)
	slowClientOne <- Event{Type: "queued"}
	slowClientTwo <- Event{Type: "queued"}
	hub.AddClient(readyClient)
	hub.AddClient(slowClientOne)
	hub.AddClient(slowClientTwo)

	var gotEventType string
	var gotDropped int
	hub.warnSlowClients = func(eventType string, dropped int) {
		gotEventType = eventType
		gotDropped = dropped
	}

	want := Event{Type: "log.created", ID: "42"}
	hub.Broadcast(want)

	select {
	case got := <-readyClient:
		if got != want {
			t.Fatalf("ready client event = %#v; want %#v", got, want)
		}
	default:
		t.Fatal("ready client did not receive broadcast immediately")
	}
	if gotEventType != want.Type || gotDropped != 2 {
		t.Fatalf("drop warning = (%q, %d); want (%q, 2)", gotEventType, gotDropped, want.Type)
	}
}

func TestBroadcastConcurrentWithRemoveIsSafe(t *testing.T) {
	t.Parallel()

	for range 100 {
		hub := NewHub()
		client := make(chan Event, 1)
		hub.AddClient(client)

		var wg sync.WaitGroup
		wg.Go(func() { hub.Broadcast(Event{Type: "test"}) })
		wg.Go(func() { hub.RemoveClient(client) })
		wg.Wait()
	}
}

func waitForCompletion(t *testing.T, operation string, done <-chan struct{}) {
	t.Helper()
	timer := time.NewTimer(time.Second)
	t.Cleanup(func() { timer.Stop() })
	select {
	case <-done:
	case <-timer.C:
		t.Fatalf("%s blocked during Broadcast", operation)
	}
}
