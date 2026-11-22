package cron

import (
	"context"
	"fmt"
	"time"
)

// Task represents a function that can be executed by the scheduler.
type Task func(ctx context.Context) error

// CronScheduler runs tasks based on a CRON expression.
type CronScheduler struct {
	cronExpr       string
	schedule       *cronSchedule
	maxRetries     int
	maxBackoff     time.Duration
	initialBackoff time.Duration
	backoffFactor  float64
	logger         func(message string)
}

// Option configures CronScheduler behavior.
type Option func(*CronScheduler)

// NewCronScheduler creates a new CRON scheduler with the given expression.
func NewCronScheduler(cronExpr string, opts ...Option) *CronScheduler {
	cs := &CronScheduler{
		cronExpr:       cronExpr,
		maxRetries:     0,
		maxBackoff:     time.Minute,
		initialBackoff: time.Second,
		backoffFactor:  2,
	}
	for _, opt := range opts {
		opt(cs)
	}
	return cs
}

// WithMaxRetries sets the maximum number of retries for failed tasks.
func WithMaxRetries(maxRetries int) Option {
	return func(cs *CronScheduler) {
		cs.maxRetries = maxRetries
	}
}

// WithMaxBackoff sets the maximum backoff duration for the CronScheduler.
func WithMaxBackoff(maxBackoff time.Duration) Option {
	return func(cs *CronScheduler) {
		cs.maxBackoff = maxBackoff
	}
}

// WithInitialBackoff sets the initial backoff duration for the CronScheduler.
func WithInitialBackoff(initialBackoff time.Duration) Option {
	return func(cs *CronScheduler) {
		cs.initialBackoff = initialBackoff
	}
}

// WithBackoffFactor sets the factor by which the backoff duration grows after
// each failed attempt. The factor must be positive (default 2).
func WithBackoffFactor(factor float64) Option {
	return func(cs *CronScheduler) {
		cs.backoffFactor = factor
	}
}

// WithLogging registers a callback for logging scheduler events. The callback
// receives a single pre-formatted message string for each event (next run
// time, retries, failures, cancellation).
//
// Without WithLogging no logging is performed.
func WithLogging(logger func(message string)) Option {
	return func(cs *CronScheduler) {
		cs.logger = logger
	}
}

// Run executes the task based on the CRON expression.
// It blocks until the context is canceled.
func (cs *CronScheduler) Run(ctx context.Context, task Task) error {
	// Parse the CRON expression once and reuse it for every scheduling step.
	schedule, err := cs.getSchedule()
	if err != nil {
		return fmt.Errorf("failed to parse CRON expression: %w", err)
	}

	nextRun, err := schedule.nextRunTime(time.Now())
	if err != nil {
		return fmt.Errorf("failed to calculate next run time: %w", err)
	}

	cs.logf("CronScheduler: next run at %s (expression: %s).", nextRun.Format("2006-01-02 15:04:05"), cs.cronExpr)

	for {
		timer := time.NewTimer(time.Until(nextRun))
		select {
		case <-ctx.Done():
			timer.Stop()
			cs.logf("CronScheduler: context cancelled, stopping.")
			return ctx.Err()
		case <-timer.C:
			if err := cs.runWithBackoff(ctx, task); err != nil {
				cs.logf("CronScheduler: task failed after retries: %v", err)
			}
			nextRun, err = schedule.nextRunTime(time.Now())
			if err != nil {
				cs.logf("CronScheduler: failed to calculate next run time: %v", err)
				// Fallback to running every minute
				nextRun = time.Now().Add(time.Minute)
			}
			cs.logf("CronScheduler: next run at %s.", nextRun.Format("2006-01-02 15:04:05"))
		}
	}
}

// runWithBackoff executes the task with exponential backoff on failure.
// It retries up to maxRetries times, growing the backoff between attempts by
// the configured backoff factor (capped at maxBackoff), and aborts early when
// the context is cancelled.
func (cs *CronScheduler) runWithBackoff(ctx context.Context, task Task) error {
	var lastErr error
	backoff := cs.initialBackoff

	for attempt := 0; attempt <= cs.maxRetries; attempt++ {
		if attempt > 0 {
			cs.logf("CronScheduler: retry attempt %d/%d after %v backoff.", attempt+1, cs.maxRetries+1, backoff)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		if err := task(ctx); err != nil {
			lastErr = err
			cs.logf("CronScheduler: task failed (attempt %d/%d): %v", attempt+1, cs.maxRetries+1, err)
			// Calculate next backoff with exponential growth.
			backoff = calculateBackoff(backoff, cs.backoffFactor, cs.maxBackoff)
			continue
		}

		if attempt > 0 {
			cs.logf("CronScheduler: task succeeded on attempt %d/%d.", attempt+1, cs.maxRetries+1)
		}
		return nil
	}

	return lastErr
}

// logf formats the message and forwards the resulting string to the
// configured logging callback, if any.
func (cs *CronScheduler) logf(format string, args ...any) {
	if cs.logger != nil {
		cs.logger(fmt.Sprintf(format, args...))
	}
}

// calculateBackoff calculates the next backoff duration by multiplying the
// current duration by the growth factor, capped at max. A non-positive factor
// produces the maximum backoff, as no valid growth can be applied.
func calculateBackoff(current time.Duration, factor float64, max time.Duration) time.Duration {
	if factor <= 0 {
		return max
	}
	next := time.Duration(float64(current) * factor)
	if next > max {
		return max
	}
	return next
}

// getSchedule returns the parsed schedule, parsing it only once per scheduler.
func (cs *CronScheduler) getSchedule() (*cronSchedule, error) {
	if cs.schedule != nil {
		return cs.schedule, nil
	}
	schedule, err := parseCronExpression(cs.cronExpr)
	if err != nil {
		return nil, err
	}
	cs.schedule = &schedule
	return cs.schedule, nil
}
