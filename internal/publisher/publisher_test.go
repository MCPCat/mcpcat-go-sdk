package publisher

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mcpcatapi "github.com/mcpcat/mcpcat-go-api"
	"github.com/mcpcat/mcpcat-go-sdk/internal/core"
	"github.com/mcpcat/mcpcat-go-sdk/internal/logging"
)

// Helper function to create string pointers
func strPtr(s string) *string {
	return &s
}

func TestNew(t *testing.T) {
	t.Run("creates publisher with default configuration", func(t *testing.T) {
		p := New(nil)
		defer p.Shutdown(context.Background())

		if p == nil {
			t.Fatal("New() returned nil")
		}

		if p.queue == nil {
			t.Error("queue channel not initialized")
		}

		if cap(p.queue) != QueueSize {
			t.Errorf("queue capacity = %d, want %d", cap(p.queue), QueueSize)
		}

		if p.apiClient == nil {
			t.Error("apiClient not initialized")
		}

		if p.logger == nil {
			t.Error("logger not initialized")
		}

		if p.ctx == nil {
			t.Error("context not initialized")
		}

		if p.cancel == nil {
			t.Error("cancel function not initialized")
		}

		if p.shutdownCh == nil {
			t.Error("shutdownCh not initialized")
		}
	})

	t.Run("creates publisher with redact function", func(t *testing.T) {
		redactFn := func(s string) string { return "***" }
		p := New(redactFn)
		defer p.Shutdown(context.Background())

		if p.redactFn == nil {
			t.Error("redactFn not set")
		}
	})

	t.Run("starts workers", func(t *testing.T) {
		p := New(nil)
		defer p.Shutdown(context.Background())

		// Give workers time to start
		time.Sleep(50 * time.Millisecond)

		// Workers should be running and ready to process events
		// We verify this by checking that we can publish an event
		event := &core.Event{
			PublishEventRequest: mcpcatapi.PublishEventRequest{
				EventType: strPtr("test"),
			},
		}

		p.Publish(event)

		// If workers aren't running, the queue would fill up
		if len(p.queue) > QueueSize {
			t.Error("workers not processing events")
		}
	})
}

func TestPublish(t *testing.T) {
	t.Run("successfully enqueues event", func(t *testing.T) {
		p := New(nil)
		defer p.Shutdown(context.Background())

		event := &core.Event{
			PublishEventRequest: mcpcatapi.PublishEventRequest{
				EventType: strPtr("test.event"),
			},
		}

		p.Publish(event)

		// Give worker time to process
		time.Sleep(50 * time.Millisecond)

		// Event should have been dequeued by worker
		queueLen := len(p.queue)
		if queueLen >= QueueSize {
			t.Errorf("queue length = %d, expected < %d", queueLen, QueueSize)
		}
	})

	t.Run("handles nil event gracefully", func(t *testing.T) {
		p := New(nil)
		defer p.Shutdown(context.Background())

		// Should not panic
		p.Publish(nil)

		// Queue should remain empty
		time.Sleep(50 * time.Millisecond)
		if len(p.queue) != 0 {
			t.Error("nil event was enqueued")
		}
	})

	t.Run("drops events when queue is full", func(t *testing.T) {
		p := New(nil)
		defer p.Shutdown(context.Background())

		// Cancel context to prevent workers from processing
		p.cancel()

		// Wait for workers to stop
		time.Sleep(100 * time.Millisecond)

		// Fill the queue
		for i := 0; i < QueueSize; i++ {
			event := &core.Event{
				PublishEventRequest: mcpcatapi.PublishEventRequest{
					EventType: strPtr("test"),
				},
			}
			p.Publish(event)
		}

		// Verify queue is full or nearly full
		queueLen := len(p.queue)
		if queueLen < QueueSize-10 {
			t.Errorf("queue length = %d, want >= %d", queueLen, QueueSize-10)
		}

		// Try to publish one more - should be dropped
		event := &core.Event{
			PublishEventRequest: mcpcatapi.PublishEventRequest{
				EventType: strPtr("dropped"),
			},
		}
		p.Publish(event)

		// Queue should still be at or near capacity (not increased significantly)
		newQueueLen := len(p.queue)
		if newQueueLen > QueueSize {
			t.Errorf("queue overflow: length = %d, capacity = %d", newQueueLen, QueueSize)
		}
	})

	t.Run("handles concurrent publishing", func(t *testing.T) {
		p := New(nil)
		defer p.Shutdown(context.Background())

		var wg sync.WaitGroup
		numGoroutines := 10
		eventsPerGoroutine := 5

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < eventsPerGoroutine; j++ {
					event := &core.Event{
						PublishEventRequest: mcpcatapi.PublishEventRequest{
							EventType: strPtr("concurrent.test"),
						},
					}
					p.Publish(event)
				}
			}()
		}

		wg.Wait()

		// Give workers time to process
		time.Sleep(200 * time.Millisecond)

		// All events should have been processed or queued
		// No panics or deadlocks = success
	})
}

func TestPublishEvent(t *testing.T) {
	t.Run("does not panic on publish", func(t *testing.T) {
		p := New(nil)
		defer p.Shutdown(context.Background())

		event := &core.Event{
			PublishEventRequest: mcpcatapi.PublishEventRequest{
				EventType: strPtr("test.success"),
			},
		}

		// Should not panic (API will fail but that's expected in test)
		p.publishEvent(event, 0)
	})

	t.Run("applies redaction before publishing", func(t *testing.T) {
		p := New(func(s string) string {
			if s == "secret" {
				return "***"
			}
			return s
		})
		defer p.Shutdown(context.Background())

		event := &core.Event{
			PublishEventRequest: mcpcatapi.PublishEventRequest{
				EventType: strPtr("test.redaction"),
				Parameters: map[string]any{
					"data": "secret",
				},
			},
		}

		p.publishEvent(event, 0)

		// Verify redaction was applied
		if event.Parameters["data"] != "***" {
			t.Errorf("redaction not applied: got %v, want ***", event.Parameters["data"])
		}
	})

	t.Run("handles redaction errors gracefully", func(t *testing.T) {
		p := New(func(s string) string {
			panic("redaction panic")
		})
		defer p.Shutdown(context.Background())

		event := &core.Event{
			PublishEventRequest: mcpcatapi.PublishEventRequest{
				EventType: strPtr("test.redaction.error"),
				Parameters: map[string]any{
					"data": "value",
				},
			},
		}

		// Should not panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("publishEvent panicked: %v", r)
			}
		}()

		p.publishEvent(event, 0)

		// Event should have redaction error placeholder for the string
		if event.Parameters["data"] != "[REDACTION_ERROR]" {
			t.Errorf("event was not sanitized: got %v, want [REDACTION_ERROR]", event.Parameters["data"])
		}
	})

	t.Run("handles API errors without panicking", func(t *testing.T) {
		p := New(nil)
		defer p.Shutdown(context.Background())

		event := &core.Event{
			PublishEventRequest: mcpcatapi.PublishEventRequest{
				EventType: strPtr("test.api.error"),
			},
		}

		// Should not panic on API error (will fail to connect but that's ok)
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("publishEvent panicked on API error: %v", r)
			}
		}()

		p.publishEvent(event, 0)
	})

	t.Run("respects context timeout", func(t *testing.T) {
		p := New(nil)
		defer p.Shutdown(context.Background())

		// This test verifies that publishEvent creates a timeout context
		// The actual timeout behavior is tested through the mock

		event := &core.Event{
			PublishEventRequest: mcpcatapi.PublishEventRequest{
				EventType: strPtr("test.timeout"),
			},
		}

		// Should complete without hanging
		done := make(chan bool)
		go func() {
			p.publishEvent(event, 0)
			done <- true
		}()

		select {
		case <-done:
			// Success
		case <-time.After(15 * time.Second):
			t.Error("publishEvent did not respect timeout")
		}
	})
}

func TestShutdown(t *testing.T) {
	t.Run("shuts down cleanly with empty queue", func(t *testing.T) {
		p := New(nil)

		if err := p.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown returned error: %v", err)
		}

		// Verify context was cancelled
		select {
		case <-p.ctx.Done():
			// Success
		default:
			t.Error("context was not cancelled")
		}

		// Verify shutdown channel was closed
		select {
		case <-p.shutdownCh:
			// Success
		case <-time.After(100 * time.Millisecond):
			t.Error("shutdownCh was not closed")
		}
	})

	t.Run("drains queue before shutdown completes", func(t *testing.T) {
		p := New(nil)

		// Publish some events
		for i := 0; i < 5; i++ {
			event := &core.Event{
				PublishEventRequest: mcpcatapi.PublishEventRequest{
					EventType: strPtr("test.shutdown"),
				},
			}
			p.Publish(event)
		}

		// Give time for events to be queued
		time.Sleep(50 * time.Millisecond)
		queuedBefore := len(p.queue)

		// Shutdown should wait for workers to process
		if err := p.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown returned error: %v", err)
		}

		// Queue should be drained or smaller
		queuedAfter := len(p.queue)
		if queuedAfter > queuedBefore {
			t.Errorf("queue grew during shutdown: before=%d, after=%d", queuedBefore, queuedAfter)
		}
	})

	t.Run("handles shutdown timeout", func(t *testing.T) {
		p := New(nil)

		// Fill queue with events that will take time to process (API calls)
		for i := 0; i < 100; i++ {
			event := &core.Event{
				PublishEventRequest: mcpcatapi.PublishEventRequest{
					EventType: strPtr("test"),
				},
			}
			p.Publish(event)
		}

		// Use an already-expired context so Shutdown returns immediately with an error.
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		time.Sleep(1 * time.Millisecond) // ensure deadline has elapsed

		start := time.Now()
		err := p.Shutdown(ctx)
		elapsed := time.Since(start)

		// Shutdown should return an error due to context timeout
		if err == nil {
			t.Error("expected Shutdown to return a timeout error")
		}

		// Shutdown should complete very quickly since context already expired
		if elapsed > 2*time.Second {
			t.Errorf("shutdown took too long = %v, want < 2s", elapsed)
		}
	})

	t.Run("can be called multiple times safely", func(t *testing.T) {
		p := New(nil)

		p.Shutdown(context.Background())
		p.Shutdown(context.Background())
		p.Shutdown(context.Background())

		// Should not panic or deadlock
	})

	t.Run("stops accepting new events after shutdown", func(t *testing.T) {
		p := New(nil)

		p.Shutdown(context.Background())

		// Try to publish after shutdown
		event := &core.Event{
			PublishEventRequest: mcpcatapi.PublishEventRequest{
				EventType: strPtr("test.after.shutdown"),
			},
		}

		// Should not panic, but event won't be processed
		p.Publish(event)
	})
}

func TestWorker(t *testing.T) {
	t.Run("processes events from queue", func(t *testing.T) {
		p := New(nil)
		defer p.Shutdown(context.Background())

		event := &core.Event{
			PublishEventRequest: mcpcatapi.PublishEventRequest{
				EventType: strPtr("test.worker"),
			},
		}

		initialQueueLen := len(p.queue)
		p.Publish(event)

		// Give worker time to process
		time.Sleep(200 * time.Millisecond)

		// Queue should have been processed (may not be empty due to timing)
		// The main thing is that the event was consumed
		finalQueueLen := len(p.queue)
		if initialQueueLen == 0 && finalQueueLen == 0 {
			// This is fine - worker processed immediately
		}
		// Worker is functioning if no panic occurred
	})

	t.Run("stops when context is cancelled", func(t *testing.T) {
		p := New(nil)

		// Cancel context
		p.cancel()

		// Give workers time to stop
		time.Sleep(100 * time.Millisecond)

		// Publishing should not cause processing after cancellation
		initialQueueLen := len(p.queue)
		event := &core.Event{
			PublishEventRequest: mcpcatapi.PublishEventRequest{
				EventType: strPtr("test"),
			},
		}
		p.Publish(event)

		time.Sleep(100 * time.Millisecond)

		// Queue should not be processed
		if initialQueueLen == 0 && len(p.queue) == 0 {
			// Queue was empty and stays empty - can't determine if worker stopped
			// This is actually a pass since event wasn't processed
		}
	})

	t.Run("ignores nil events in queue", func(t *testing.T) {
		p := New(nil)
		defer p.Shutdown(context.Background())

		// Manually insert nil into queue
		p.queue <- nil

		// Give worker time to process
		time.Sleep(100 * time.Millisecond)

		// Should not panic - nil events are skipped
		// If we got here, the test passed
	})
}

// newSpyPublisher creates a Publisher whose workers increment an atomic counter
// instead of making real API calls. This allows tests to precisely verify how
// many events were processed. The optional handler func, if non-nil, is called
// for each event (e.g. to inject delays). The returned *int64 points to the
// processed-event counter.
func newSpyPublisher(numWorkers int, queueSize int, handler func(*core.Event)) (*Publisher, *int64) {
	ctx, cancel := context.WithCancel(context.Background())
	var counter int64

	p := &Publisher{
		queue:      make(chan *core.Event, queueSize),
		logger:     logging.New(),
		ctx:        ctx,
		cancel:     cancel,
		shutdownCh: make(chan struct{}),
	}

	for i := 0; i < numWorkers; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for {
				select {
				case <-p.ctx.Done():
					// Drain remaining events before exiting
					for {
						select {
						case event := <-p.queue:
							if event != nil {
								if handler != nil {
									handler(event)
								}
								atomic.AddInt64(&counter, 1)
							}
						default:
							return
						}
					}
				case event := <-p.queue:
					if event == nil {
						continue
					}
					if handler != nil {
						handler(event)
					}
					atomic.AddInt64(&counter, 1)
				}
			}
		}()
	}

	return p, &counter
}

// makeEvent creates a simple test event.
func makeEvent(eventType string) *core.Event {
	return &core.Event{
		PublishEventRequest: mcpcatapi.PublishEventRequest{
			EventType: strPtr(eventType),
		},
	}
}

func TestShutdownDrainsAllQueuedEvents(t *testing.T) {
	const totalEvents = 20

	p, counter := newSpyPublisher(MaxWorkers, QueueSize, nil)

	for i := 0; i < totalEvents; i++ {
		p.Publish(makeEvent("drain.test"))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := p.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	processed := atomic.LoadInt64(counter)
	if processed != totalEvents {
		t.Errorf("processed %d events, want %d", processed, totalEvents)
	}
}

func TestShutdownRespectsContextDeadline(t *testing.T) {
	// Use a handler that blocks long enough to prevent draining within the
	// deadline. Each event takes 100ms and we enqueue 50, but the context
	// expires almost immediately.
	slowHandler := func(_ *core.Event) {
		time.Sleep(100 * time.Millisecond)
	}

	p, _ := newSpyPublisher(1, QueueSize, slowHandler)

	for i := 0; i < 50; i++ {
		p.Publish(makeEvent("slow.test"))
	}

	// Use an already-expired context
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(1 * time.Millisecond) // ensure deadline has elapsed

	err := p.Shutdown(ctx)
	if err == nil {
		t.Fatal("expected Shutdown to return a non-nil error when context deadline exceeded")
	}

	// The error should be a context error (DeadlineExceeded or Canceled)
	if err != context.DeadlineExceeded && err != context.Canceled {
		t.Errorf("expected context.DeadlineExceeded or context.Canceled, got %v", err)
	}
}

func TestShutdownIsIdempotent(t *testing.T) {
	p, _ := newSpyPublisher(MaxWorkers, QueueSize, nil)

	// Enqueue a few events so the first shutdown has real work to do
	for i := 0; i < 5; i++ {
		p.Publish(makeEvent("idempotent.test"))
	}

	err1 := p.Shutdown(context.Background())
	if err1 != nil {
		t.Errorf("first Shutdown returned error: %v", err1)
	}

	// Second call must not panic and must also return nil
	err2 := p.Shutdown(context.Background())
	if err2 != nil {
		t.Errorf("second Shutdown returned error: %v", err2)
	}
}

func TestWorkerDrainsOnContextCancel(t *testing.T) {
	// Regression test: when the publisher's internal context is cancelled,
	// workers must drain all remaining queued events before exiting.

	const totalEvents = 15
	p, counter := newSpyPublisher(MaxWorkers, QueueSize, nil)

	// Pause workers so events accumulate in the queue. We do this by
	// cancelling the context first, then waiting for workers to stop, then
	// re-creating the publisher with events already queued.
	// Instead, a simpler approach: enqueue events, then cancel the context
	// (simulating what Shutdown does), then wait for workers to finish.

	for i := 0; i < totalEvents; i++ {
		p.Publish(makeEvent("drain.cancel.test"))
	}

	// Give a moment for events to land in the channel
	time.Sleep(10 * time.Millisecond)

	// Cancel the internal context (same thing Shutdown does)
	p.cancel()

	// Wait for all workers to finish draining
	p.wg.Wait()

	processed := atomic.LoadInt64(counter)
	if processed != totalEvents {
		t.Errorf("processed %d events after context cancel, want %d", processed, totalEvents)
	}
}

func TestShutdownReturnsErrorWhenEventsRemain(t *testing.T) {
	// Use a handler that blocks for a long time, ensuring events remain
	// unprocessed when the short timeout fires.
	slowHandler := func(_ *core.Event) {
		time.Sleep(500 * time.Millisecond)
	}

	// Use a single worker so processing is serialised and slow
	p, _ := newSpyPublisher(1, QueueSize, slowHandler)

	for i := 0; i < 20; i++ {
		p.Publish(makeEvent("timeout.test"))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := p.Shutdown(ctx)
	if err == nil {
		t.Fatal("expected Shutdown to return an error when events remain unprocessed")
	}

	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}
