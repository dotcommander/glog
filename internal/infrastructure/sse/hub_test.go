package sse

import "testing"

func TestRemoveClientAfterCloseIsIdempotent(t *testing.T) {
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
