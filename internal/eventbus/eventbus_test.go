package eventbus

import (
	"testing"
	"time"
)

func TestSubscribeReceivesPublishedEvent(t *testing.T) {
	bus := New()
	ch, unsubscribe := bus.Subscribe()
	defer unsubscribe()

	want := Event{Type: "heartbeat", Data: "1"}
	bus.Publish(want)

	select {
	case got := <-ch:
		if got != want {
			t.Errorf("got event %+v, want %+v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestPublishFansOutToAllSubscribers(t *testing.T) {
	bus := New()
	ch1, unsubscribe1 := bus.Subscribe()
	defer unsubscribe1()
	ch2, unsubscribe2 := bus.Subscribe()
	defer unsubscribe2()

	want := Event{Type: "heartbeat", Data: "1"}
	bus.Publish(want)

	for i, ch := range []<-chan Event{ch1, ch2} {
		select {
		case got := <-ch:
			if got != want {
				t.Errorf("subscriber %d: got event %+v, want %+v", i, got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out waiting for event", i)
		}
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	bus := New()
	ch, unsubscribe := bus.Subscribe()
	unsubscribe()

	bus.Publish(Event{Type: "heartbeat", Data: "1"})

	if _, ok := <-ch; ok {
		t.Error("expected channel to be closed after unsubscribe")
	}
}

func TestPublishWithNoSubscribersDoesNotBlock(t *testing.T) {
	bus := New()

	done := make(chan struct{})
	go func() {
		bus.Publish(Event{Type: "heartbeat", Data: "1"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked with no subscribers")
	}
}

func TestPublishDropsEventForFullSubscriberBuffer(t *testing.T) {
	bus := New()
	ch, unsubscribe := bus.Subscribe()
	defer unsubscribe()

	// Fill the subscriber's buffer, then publish one more than it can hold.
	for i := 0; i < subscriberBuffer+1; i++ {
		bus.Publish(Event{Type: "heartbeat", Data: "overflow"})
	}

	drained := 0
	for {
		select {
		case <-ch:
			drained++
		default:
			if drained != subscriberBuffer {
				t.Errorf("got %d buffered events, want %d", drained, subscriberBuffer)
			}
			return
		}
	}
}
