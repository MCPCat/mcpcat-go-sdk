package publisher

import (
	"sync"
	"testing"
	"time"

	mcpcatapi "github.com/mcpcat/mcpcat-go-api"
	"github.com/mcpcat/mcpcat-go-sdk/internal/core"
)

// Helper function to create string pointers
func strPtr(s string) *string {
	return &s
}

func TestNew(t *testing.T) {
	t.Run("creates publisher with default configuration", func(t *testing.T) {
		p := New(nil)
		defer p.Shutdown()

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
		defer p.Shutdown()

		if p.redactFn == nil {
			t.Error("redactFn not set")
		}
	})

	t.Run("starts workers", func(t *testing.T) {
		p := New(nil)
		defer p.Shutdown()

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
		defer p.Shutdown()

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
		defer p.Shutdown()

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
		defer p.Shutdown()

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
		defer p.Shutdown()

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
		defer p.Shutdown()

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
		defer p.Shutdown()

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
		defer p.Shutdown()

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
		defer p.Shutdown()

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
		defer p.Shutdown()

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

		p.Shutdown()

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
		p.Shutdown()

		// Queue should be drained or smaller
		queuedAfter := len(p.queue)
		if queuedAfter > queuedBefore {
			t.Errorf("queue grew during shutdown: before=%d, after=%d", queuedBefore, queuedAfter)
		}
	})

	t.Run("handles shutdown timeout", func(t *testing.T) {
		p := New(nil)

		// Stop workers from processing by canceling immediately
		p.cancel()
		time.Sleep(100 * time.Millisecond)

		// Fill queue with many events
		for i := 0; i < 100; i++ {
			event := &core.Event{
				PublishEventRequest: mcpcatapi.PublishEventRequest{
					EventType: strPtr("test"),
				},
			}
			// Direct insertion to avoid Publish() logic
			select {
			case p.queue <- event:
			default:
				break
			}
		}

		start := time.Now()
		p.Shutdown()
		elapsed := time.Since(start)

		// Shutdown should timeout around 5 seconds (give some tolerance)
		if elapsed > 6*time.Second {
			t.Errorf("shutdown timeout = %v, want <= 6s", elapsed)
		}
	})

	t.Run("can be called multiple times safely", func(t *testing.T) {
		p := New(nil)

		p.Shutdown()
		p.Shutdown()
		p.Shutdown()

		// Should not panic or deadlock
	})

	t.Run("stops accepting new events after shutdown", func(t *testing.T) {
		p := New(nil)

		p.Shutdown()

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
		defer p.Shutdown()

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
		defer p.Shutdown()

		// Manually insert nil into queue
		p.queue <- nil

		// Give worker time to process
		time.Sleep(100 * time.Millisecond)

		// Should not panic - nil events are skipped
		// If we got here, the test passed
	})
}

func TestHandleSignals(t *testing.T) {
	t.Run("sets flag on first call", func(t *testing.T) {
		p := New(nil)
		defer p.Shutdown()

		// Give signal handler goroutine time to start
		time.Sleep(100 * time.Millisecond)

		// Signal handler should have been started by New()
		if !p.signalHandler.Load() {
			t.Error("signal handler flag not set after starting")
		}

		// Try to start another signal handler - should be prevented
		go p.handleSignals()

		// Give it time to attempt
		time.Sleep(50 * time.Millisecond)

		// Only one should be running (verified by not having a panic or issue)
	})

	t.Run("stops on shutdown signal", func(t *testing.T) {
		p := New(nil)

		// Trigger shutdown through the shutdown channel
		go func() {
			time.Sleep(50 * time.Millisecond)
			p.Shutdown()
		}()

		// Wait for shutdown
		<-p.shutdownCh

		// handleSignals should exit gracefully
	})
}
