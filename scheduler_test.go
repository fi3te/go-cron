package cron

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewCronScheduler(t *testing.T) {
	cs := NewCronScheduler("0 * * * *")
	if cs == nil {
		t.Fatal("NewCronScheduler returned nil")
	}
	if cs.cronExpr != "0 * * * *" {
		t.Errorf("expected cronExpr %q, got %q", "0 * * * *", cs.cronExpr)
	}
}

func TestCronScheduler_Options(t *testing.T) {
	cs := NewCronScheduler(
		"0 * * * *",
		WithMaxRetries(5),
		WithMaxBackoff(5*time.Minute),
		WithInitialBackoff(500*time.Millisecond),
		WithBackoffFactor(3),
	)

	if cs.maxRetries != 5 {
		t.Errorf("expected maxRetries 5, got %d", cs.maxRetries)
	}
	if cs.maxBackoff != 5*time.Minute {
		t.Errorf("expected maxBackoff %v, got %v", 5*time.Minute, cs.maxBackoff)
	}
	if cs.initialBackoff != 500*time.Millisecond {
		t.Errorf("expected initialBackoff %v, got %v", 500*time.Millisecond, cs.initialBackoff)
	}
	if cs.backoffFactor != 3 {
		t.Errorf("expected backoffFactor 3, got %v", cs.backoffFactor)
	}
}

func TestCronScheduler_OptionsDefaults(t *testing.T) {
	cs := NewCronScheduler("0 * * * *")

	if cs.maxRetries != 0 {
		t.Errorf("expected default maxRetries 0, got %d", cs.maxRetries)
	}
	if cs.maxBackoff != time.Minute {
		t.Errorf("expected default maxBackoff %v, got %v", time.Minute, cs.maxBackoff)
	}
	if cs.initialBackoff != time.Second {
		t.Errorf("expected default initialBackoff %v, got %v", time.Second, cs.initialBackoff)
	}
	if cs.backoffFactor != 2 {
		t.Errorf("expected default backoffFactor 2, got %v", cs.backoffFactor)
	}
	if cs.logger != nil {
		t.Error("expected no logging callback by default")
	}
}

func TestCronScheduler_WithBackoffFactor(t *testing.T) {
	cs := NewCronScheduler("0 * * * *", WithBackoffFactor(4))
	if cs.backoffFactor != 4 {
		t.Errorf("expected backoffFactor 4, got %v", cs.backoffFactor)
	}
}

func TestCalculateBackoff(t *testing.T) {
	tests := []struct {
		name    string
		current time.Duration
		factor  float64
		max     time.Duration
		want    time.Duration
	}{
		{"default doubling", 10 * time.Millisecond, 2, time.Minute, 20 * time.Millisecond},
		{"factor 3", 10 * time.Millisecond, 3, time.Minute, 30 * time.Millisecond},
		{"factor 1.5", 10 * time.Millisecond, 1.5, time.Minute, 15 * time.Millisecond},
		{"capped at max", 30 * time.Millisecond, 2, 50 * time.Millisecond, 50 * time.Millisecond},
		{"non-positive factor returns max", 10 * time.Millisecond, 0, 50 * time.Millisecond, 50 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := calculateBackoff(tt.current, tt.factor, tt.max); got != tt.want {
				t.Errorf("calculateBackoff(%v, %v, %v) = %v, want %v", tt.current, tt.factor, tt.max, got, tt.want)
			}
		})
	}
}

func TestCronScheduler_RunWithBackoff_NoRetriesByDefault(t *testing.T) {
	cs := NewCronScheduler(
		"* * * * *",
		WithInitialBackoff(5*time.Millisecond),
		WithMaxBackoff(20*time.Millisecond),
	)

	callCount := 0
	task := func(ctx context.Context) error {
		callCount++
		return errors.New("always fails")
	}

	err := cs.runWithBackoff(context.Background(), task)
	if err == nil {
		t.Fatal("expected error after failing the single attempt")
	}
	if callCount != 1 {
		t.Errorf("expected exactly 1 call with maxRetries=0, got %d", callCount)
	}
}

func TestCronScheduler_WithLogging(t *testing.T) {
	var gotMessage string
	logger := func(message string) {
		gotMessage = message
	}

	cs := NewCronScheduler("0 * * * *", WithLogging(logger))
	if cs.logger == nil {
		t.Fatal("expected logging callback to be registered")
	}
	cs.logger("formatted message")
	if gotMessage != "formatted message" {
		t.Errorf("expected message %q, got %q", "formatted message", gotMessage)
	}
}

func TestCronScheduler_LogfFormatsBeforeCallback(t *testing.T) {
	var gotMessage string
	logger := func(message string) {
		gotMessage = message
	}

	cs := NewCronScheduler("0 * * * *", WithLogging(logger))
	cs.logf("CronScheduler: task failed (attempt %d): %v", 3, errors.New("boom"))
	if !strings.Contains(gotMessage, "task failed (attempt 3): boom") {
		t.Errorf("expected formatted message containing the failure, got %q", gotMessage)
	}
}

func TestCronScheduler_RunWithBackoff_LoggingCallback(t *testing.T) {
	var messages []string
	logger := func(message string) {
		messages = append(messages, message)
	}

	cs := NewCronScheduler(
		"* * * * *",
		WithLogging(logger),
		WithMaxRetries(3),
		WithInitialBackoff(5*time.Millisecond),
		WithMaxBackoff(20*time.Millisecond),
	)

	callCount := 0
	task := func(ctx context.Context) error {
		callCount++
		if callCount <= 1 {
			return errors.New("boom")
		}
		return nil
	}

	if err := cs.runWithBackoff(context.Background(), task); err != nil {
		t.Fatalf("runWithBackoff returned unexpected error: %v", err)
	}

	found := false
	for _, m := range messages {
		if strings.Contains(m, "task failed") && strings.Contains(m, "boom") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a 'task failed' log message with the task error, got %v", messages)
	}
}

func TestCronScheduler_GetScheduleInvalidExpression(t *testing.T) {
	cs := NewCronScheduler("invalid")
	if _, err := cs.getSchedule(); err == nil {
		t.Error("expected error for invalid expression")
	}
}

func TestCronScheduler_Run_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	task := func(ctx context.Context) error {
		callCount++
		return nil
	}

	cs := NewCronScheduler("* * * * *")

	var runErr error
	done := make(chan struct{})
	go func() {
		runErr = cs.Run(ctx, task)
		close(done)
	}()

	// Let the scheduler start, then cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		if !errors.Is(runErr, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", runErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

func TestCronScheduler_Run_InvalidExpression(t *testing.T) {
	ctx := context.Background()
	task := func(ctx context.Context) error { return nil }

	cs := NewCronScheduler("not a cron")
	if err := cs.Run(ctx, task); err == nil {
		t.Error("expected error for invalid CRON expression")
	}
}

func TestCronScheduler_RunWithBackoff_SuccessOnFirstAttempt(t *testing.T) {
	cs := NewCronScheduler(
		"* * * * *",
		WithMaxRetries(3),
		WithInitialBackoff(10*time.Millisecond),
	)

	callCount := 0
	task := func(ctx context.Context) error {
		callCount++
		return nil
	}

	if err := cs.runWithBackoff(context.Background(), task); err != nil {
		t.Fatalf("runWithBackoff returned unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected exactly 1 call, got %d", callCount)
	}
}

func TestCronScheduler_RunWithBackoff_SuccessAfterRetries(t *testing.T) {
	cs := NewCronScheduler(
		"* * * * *",
		WithMaxRetries(3),
		WithInitialBackoff(5*time.Millisecond),
		WithMaxBackoff(20*time.Millisecond),
	)

	callCount := 0
	task := func(ctx context.Context) error {
		callCount++
		if callCount <= 2 {
			return errors.New("simulated failure")
		}
		return nil
	}

	if err := cs.runWithBackoff(context.Background(), task); err != nil {
		t.Fatalf("runWithBackoff returned unexpected error: %v", err)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls (2 failures + 1 success), got %d", callCount)
	}
}

func TestCronScheduler_RunWithBackoff_GivesUpAfterMaxRetries(t *testing.T) {
	cs := NewCronScheduler(
		"* * * * *",
		WithMaxRetries(2),
		WithInitialBackoff(5*time.Millisecond),
		WithMaxBackoff(20*time.Millisecond),
	)

	callCount := 0
	task := func(ctx context.Context) error {
		callCount++
		return errors.New("always fails")
	}

	err := cs.runWithBackoff(context.Background(), task)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if !strings.Contains(err.Error(), "always fails") {
		t.Errorf("expected the task error to be returned, got %v", err)
	}
	// One initial attempt plus maxRetries retries.
	if callCount != cs.maxRetries+1 {
		t.Errorf("expected %d calls (maxRetries+1), got %d", cs.maxRetries+1, callCount)
	}
}

func TestCronScheduler_RunWithBackoff_ContextCancelledDuringBackoff(t *testing.T) {
	cs := NewCronScheduler(
		"* * * * *",
		WithMaxRetries(5),
		WithInitialBackoff(100*time.Second),
		WithMaxBackoff(200*time.Second),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	task := func(ctx context.Context) error {
		return errors.New("always fails")
	}

	done := make(chan error, 1)
	go func() {
		done <- cs.runWithBackoff(ctx, task)
	}()

	// Cancel while the scheduler is waiting on the backoff timer.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected context cancellation error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runWithBackoff did not return after context cancellation")
	}
}
