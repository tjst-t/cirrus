package jobqueue

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// HandlerFunc is the signature for a job type handler.
// Returning a non-nil error causes the job to be marked as failed.
type HandlerFunc func(ctx context.Context, job *Job) error

// HandlerRegistrar is implemented by services that register job handlers
// with a Dispatcher. Declared here so callers can use explicit type assertions
// that fail loudly at startup rather than silently skipping registration.
type HandlerRegistrar interface {
	RegisterHandlers(d *Dispatcher)
}

// Option is a functional option for NewDispatcher.
type Option func(*Dispatcher)

// WithPollInterval sets the polling interval between dequeue attempts.
// Default is 1 second.
func WithPollInterval(d time.Duration) Option {
	return func(disp *Dispatcher) { disp.pollInterval = d }
}

// Dispatcher fans out dequeued jobs to registered handlers using N worker goroutines.
type Dispatcher struct {
	queue        Queue
	handlers     map[string]HandlerFunc
	workerCount  int
	pollInterval time.Duration
	logger       *slog.Logger
	mu           sync.RWMutex
	notify       chan struct{}
}

// NewDispatcher creates a Dispatcher.
// workerCount is the number of concurrent worker goroutines.
// logger may be nil (defaults to the default slog logger).
// opts can be used to customise behaviour (e.g. WithPollInterval).
func NewDispatcher(queue Queue, workerCount int, logger *slog.Logger, opts ...Option) *Dispatcher {
	if workerCount <= 0 {
		workerCount = 1
	}
	if logger == nil {
		logger = slog.Default()
	}
	d := &Dispatcher{
		queue:        queue,
		handlers:     make(map[string]HandlerFunc),
		workerCount:  workerCount,
		pollInterval: time.Second,
		logger:       logger,
		notify:       make(chan struct{}, 1),
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Notify wakes up idle workers so they attempt a dequeue without waiting for the
// next poll interval. Safe to call from any goroutine; non-blocking.
func (d *Dispatcher) Notify() {
	select {
	case d.notify <- struct{}{}:
	default:
	}
}

// Register associates a handler with a job type.
// Must be called before Start.
func (d *Dispatcher) Register(jobType string, h HandlerFunc) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[jobType] = h
}

// Start launches the worker goroutines. It blocks until ctx is cancelled.
// All workers are started and the function returns only after all have exited.
func (d *Dispatcher) Start(ctx context.Context) {
	d.mu.RLock()
	jobTypes := make([]string, 0, len(d.handlers))
	for t := range d.handlers {
		jobTypes = append(jobTypes, t)
	}
	d.mu.RUnlock()

	if len(jobTypes) == 0 {
		d.logger.Warn("jobqueue dispatcher: no handlers registered; not starting workers")
		<-ctx.Done()
		return
	}

	var wg sync.WaitGroup
	for i := range d.workerCount {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			d.runWorker(ctx, workerID, jobTypes)
		}(i)
	}
	wg.Wait()
}

func (d *Dispatcher) runWorker(ctx context.Context, id int, jobTypes []string) {
	log := d.logger.With("worker_id", id)
	for {
		select {
		case <-ctx.Done():
			log.Debug("jobqueue worker: shutting down")
			return
		default:
		}

		job, err := d.queue.Dequeue(ctx, jobTypes)
		if err != nil {
			log.Error("jobqueue worker: dequeue error", "error", err)
			// Back off to avoid tight error loops.
			select {
			case <-ctx.Done():
				return
			case <-time.After(d.pollInterval):
			}
			continue
		}

		if job == nil {
			// No work available; wait for a notify signal or poll interval before retrying.
			select {
			case <-ctx.Done():
				return
			case <-d.notify:
			case <-time.After(d.pollInterval):
			}
			continue
		}

		log.Info("jobqueue worker: processing job", "job_id", job.ID, "job_type", job.Type)
		d.mu.RLock()
		handler := d.handlers[job.Type]
		d.mu.RUnlock()

		if handler == nil {
			msg := fmt.Sprintf("no handler registered for job type %q", job.Type)
			log.Error("jobqueue worker: "+msg, "job_id", job.ID)
			if ferr := d.queue.Fail(ctx, job.ID, msg); ferr != nil {
				log.Error("jobqueue worker: fail job", "job_id", job.ID, "error", ferr)
			}
			continue
		}

		if herr := handler(ctx, job); herr != nil {
			log.Error("jobqueue worker: handler error", "job_id", job.ID, "error", herr)
			if ferr := d.queue.Fail(ctx, job.ID, herr.Error()); ferr != nil {
				log.Error("jobqueue worker: fail job after handler error", "job_id", job.ID, "error", ferr)
			}
			continue
		}

		if cerr := d.queue.Complete(ctx, job.ID); cerr != nil {
			log.Error("jobqueue worker: complete job", "job_id", job.ID, "error", cerr)
		}
	}
}
