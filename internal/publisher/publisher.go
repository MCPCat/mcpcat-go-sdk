package publisher

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	mcpcatapi "github.com/mcpcat/mcpcat-go-api"
	"github.com/mcpcat/mcpcat-go-sdk/internal/core"
	"github.com/mcpcat/mcpcat-go-sdk/internal/logging"
	"github.com/mcpcat/mcpcat-go-sdk/internal/redaction"
)

// Publisher handles asynchronous event publishing to the MCPCat API
type Publisher struct {
	queue         chan *core.Event
	apiClient     *mcpcatapi.APIClient
	logger        *logging.Logger
	redactFn      core.RedactFunc
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
	shutdownCh    chan struct{}
	shutdownOnce  sync.Once
	signalHandler atomic.Bool
}

// New creates a new Publisher instance and starts worker goroutines
func New(redactFn core.RedactFunc) *Publisher {
	logger := logging.New()

	// Create API configuration with default URL
	cfg := mcpcatapi.NewConfiguration()
	cfg.Servers = mcpcatapi.ServerConfigurations{
		{
			URL:         DefaultAPIBaseURL,
			Description: "MCPCat API",
		},
	}

	apiClient := mcpcatapi.NewAPIClient(cfg)

	ctx, cancel := context.WithCancel(context.Background())

	p := &Publisher{
		queue:      make(chan *core.Event, QueueSize),
		apiClient:  apiClient,
		logger:     logger,
		redactFn:   redactFn,
		ctx:        ctx,
		cancel:     cancel,
		shutdownCh: make(chan struct{}),
	}

	// Start worker pool
	for i := 0; i < MaxWorkers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}

	// Start automatic signal handler for graceful shutdown
	go p.handleSignals()

	logger.Infof("Publisher started with %d workers and queue size %d", MaxWorkers, QueueSize)

	return p
}

// handleSignals listens for OS signals and triggers graceful shutdown
func (p *Publisher) handleSignals() {
	if !p.signalHandler.CompareAndSwap(false, true) {
		// Signal handler already running
		return
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		p.logger.Infof("Received signal %v, draining event queue...", sig)
		p.Shutdown()
		// Give logger time to flush before exiting
		time.Sleep(100 * time.Millisecond)
		os.Exit(0)
	case <-p.shutdownCh:
		// Publisher already shut down manually
		return
	}
}

// worker processes events from the queue and publishes them to the API
func (p *Publisher) worker(id int) {
	defer p.wg.Done()

	p.logger.Debugf("Worker %d started", id)

	for {
		select {
		case <-p.ctx.Done():
			p.logger.Debugf("Worker %d shutting down", id)
			return
		case event := <-p.queue:
			if event == nil {
				continue
			}

			p.publishEvent(event, id)
		}
	}
}

// publishEvent sends a single event to the MCPCat API
func (p *Publisher) publishEvent(event *core.Event, workerID int) {
	// Apply redaction if a redact function is configured
	if p.redactFn != nil {
		err := redaction.RedactEvent(event, p.redactFn)
		if err != nil {
			p.logger.Warnf("Worker %d redaction failed for event %s: %v - publishing with error placeholders",
				workerID, event.GetId(), err)
			// Event has been sanitized with error placeholders, safe to continue publishing
		} else {
			p.logger.Debugf("Worker %d applied redaction to event %s", workerID, event.GetId())
		}
	}

	// Set a reasonable timeout for the API call
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Publish event (no authentication needed - public API)
	_, resp, err := p.apiClient.EventsAPI.PublishEvent(ctx).
		PublishEventRequest(event.PublishEventRequest).
		Execute()

	if err != nil {
		p.logger.Errorf("Worker %d failed to publish event: %v", workerID, err)
		if resp != nil {
			p.logger.Debugf("Response status: %s", resp.Status)
		}
		return
	}

	if resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		p.logger.Debugf("Worker %d successfully published event %s", workerID, event.GetId())
	} else {
		p.logger.Warnf("Worker %d received unexpected status code: %d", workerID, resp.StatusCode)
	}
}

// Publish enqueues an event for publishing. If the queue is full, the newest event is dropped.
func (p *Publisher) Publish(event *core.Event) {
	if event == nil {
		return
	}

	select {
	case p.queue <- event:
		// Successfully enqueued
		p.logger.Debugf("Event %s enqueued for publishing", event.GetId())
	default:
		// Queue is full, drop the newest event
		p.logger.Warnf("Queue full, dropping event %s", event.GetId())
	}
}

// Shutdown gracefully shuts down the publisher, waiting up to 5 seconds for queued events to be published
func (p *Publisher) Shutdown() {
	p.shutdownOnce.Do(func() {
		queuedCount := len(p.queue)
		if queuedCount > 0 {
			p.logger.Infof("Publisher shutting down with %d events in queue...", queuedCount)
		} else {
			p.logger.Info("Publisher shutting down...")
		}

		// Stop accepting new events by canceling context
		p.cancel()

		// Wait for workers to finish or timeout after 5 seconds
		done := make(chan struct{})
		go func() {
			p.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			remaining := len(p.queue)
			if remaining > 0 {
				p.logger.Warnf("Shutdown complete, but %d events remain unpublished", remaining)
			} else {
				p.logger.Info("All events published successfully")
			}
		case <-time.After(5 * time.Second):
			remaining := len(p.queue)
			p.logger.Warnf("Shutdown timeout reached, %d events may not have been published", remaining)
		}

		close(p.shutdownCh)
	})
}
