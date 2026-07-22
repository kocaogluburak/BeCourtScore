package sse_test

import (
	"testing"
	"time"

	"courtscore/internal/platform/sse"
)

func TestHubSubscribeAndPublish(t *testing.T) {
	hub := sse.NewHub()

	ch, unsub := hub.Subscribe("user-1")
	defer unsub()

	event := sse.Event{Type: "test", Data: "hello"}
	hub.Publish("user-1", event)

	select {
	case got := <-ch:
		if got.Type != event.Type {
			t.Errorf("Type: got %q, want %q", got.Type, event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestHubUnsubscribe(t *testing.T) {
	hub := sse.NewHub()

	ch, unsub := hub.Subscribe("user-1")
	unsub()

	hub.Publish("user-1", sse.Event{Type: "after-unsub"})

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected closed channel after unsub")
		}
	case <-time.After(50 * time.Millisecond):
		// channel was closed synchronously by unsub, so no event arrives — pass
	}
}

func TestHubPublishOnlyToUser(t *testing.T) {
	hub := sse.NewHub()

	ch1, unsub1 := hub.Subscribe("user-1")
	ch2, unsub2 := hub.Subscribe("user-2")
	defer unsub1()
	defer unsub2()

	hub.Publish("user-1", sse.Event{Type: "private"})

	select {
	case <-ch1:
		// correct
	case <-time.After(time.Second):
		t.Fatal("user-1 did not receive event")
	}

	select {
	case ev := <-ch2:
		t.Errorf("user-2 should not receive user-1 event, got %+v", ev)
	case <-time.After(50 * time.Millisecond):
		// correct — nothing received
	}
}

func TestHubMultipleSubscribers(t *testing.T) {
	hub := sse.NewHub()

	ch1, unsub1 := hub.Subscribe("user-1")
	ch2, unsub2 := hub.Subscribe("user-1") // same user, two connections
	defer unsub1()
	defer unsub2()

	hub.Publish("user-1", sse.Event{Type: "broadcast"})

	for i, ch := range []chan sse.Event{ch1, ch2} {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Errorf("connection %d did not receive event", i+1)
		}
	}
}
