# go-cron

A lightweight and efficient CRON library for Go that lets you schedule and run
tasks based on a cron schedule. It parses standard CRON expressions, computes
the next run time, and executes your tasks with optional retry/backoff logic.

## Features

- **Standard 5-field CRON format** – Supports `*`, lists (`,`), ranges (`a-b`),
  steps (`*/n` and `a-b/n`) and combinations thereof.
- **Any Go function as a task** – Any function with the signature
  `func(ctx context.Context) error` can be scheduled.
- **Automatic next-run computation** – The scheduler runs efficiently until the
  next matching time instead of polling every minute.
- **Retry with exponential backoff** – Failed tasks are retried automatically
  with configurable initial and maximum backoff.
- **Context-based execution** – The scheduler can be stopped cleanly via
  `context.Context` cancellation.
- **Vixie cron day semantics** – OR logic between day-of-month and
  day-of-week, as known from classic cron implementations.
- **Configuration via option functions** – Functional options pattern with
  sensible defaults.

## Installation

```bash
go get github.com/fi3te/go-cron
```

Import the package:

```go
import "github.com/fi3te/go-cron"
```

## Quick Start

In the simplest case, run a task every hour on the hour:

```go
package main

import (
	"context"
	"log"

	"github.com/fi3te/go-cron"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scheduler := cron.NewCronScheduler("0 * * * *")

	err := scheduler.Run(ctx, func(ctx context.Context) error {
		log.Println("Hourly task executed")
		return nil
	})
	if err != nil {
		log.Fatalf("scheduler stopped with error: %v", err)
	}
}
```

## CRON Expression

The library supports the classic **5-field CRON format**:

```
┌─────────────  minute (0 - 59)
│ ┌───────────  hour (0 - 23)
│ │ ┌─────────  day of month (1 - 31)
│ │ │ ┌───────  month (1 - 12)
│ │ │ │ ┌─────  day of week (0 - 7)  (Sunday = 0 or 7)
│ │ │ │ │
* * * * *
```

### Supported Syntax

| Syntax    | Meaning                                  | Example                  |
|-----------|------------------------------------------|--------------------------|
| `*`       | Wildcard (any value)                     | `0 * * * *` = hourly     |
| `a,b,c`   | List of values                           | `0 9,17 * * *`           |
| `a-b`     | Range                                    | `0 9-17 * * *`           |
| `*/n`     | Step over the entire range               | `*/15 * * * *`           |
| `a-b/n`   | Step within a range                      | `30-59/10 * * * *`       |

The forms above can be freely combined, e.g. `0 9,12-14 * * 1-5`.

### Example Expressions

| CRON expression | Meaning                                  |
|-----------------|------------------------------------------|
| `* * * * *`     | Every minute                             |
| `0 * * * *`     | Every hour on the hour                   |
| `0 9 * * *`     | Daily at 09:00                           |
| `0 9 * * 1-5`   | Mon–Fri at 09:00                         |
| `*/15 * * * *`  | Every 15 minutes                         |
| `30 2 * * *`    | Daily at 02:30                           |
| `0 0 1 * *`     | Midnight on the 1st of every month       |

**Note:** For day-of-week, both 0 and 7 represent Sunday. When both
day-of-month and day-of-week are non-wildcards, OR semantics apply
(Vixie cron): the task runs if at least one of the two fields matches.

## Configuration

You can adjust the scheduler behavior via option functions. All options are
optional; if omitted, the defaults are used.

| Option                  | Default      | Description                                |
|-------------------------|--------------|--------------------------------------------|
| `WithMaxRetries(n)`     | `0`          | Maximum number of retry attempts (0 = off)  |
| `WithMaxBackoff(d)`     | `1m` (1 min) | Maximum wait time between attempts          |
| `WithInitialBackoff(d)` | `1s` (1 s)   | Initial backoff before the first retry      |
| `WithBackoffFactor(f)`  | `2`          | Growth factor for the backoff duration      |
| `WithLogging(fn)`       | none         | Register a callback for scheduler events    |

### Example with Options

```go
scheduler := cron.NewCronScheduler(
	"*/5 * * * *",
	cron.WithMaxRetries(5),
	cron.WithInitialBackoff(time.Second),
	cron.WithMaxBackoff(5*time.Minute),
	cron.WithBackoffFactor(2),
	cron.WithLogging(func(message string) {
		log.Println(message)
	}),
)
```

The backoff wait time grows between attempts by `WithBackoffFactor`
(multiplying the previous backoff), but is capped at `WithMaxBackoff`.
Retries are disabled by default (`WithMaxRetries` defaults to `0`), so a task
is executed exactly once unless `WithMaxRetries` is set to a value greater
than `0`.

## Task Signature

A task is any function with the following signature:

```go
type Task func(ctx context.Context) error
```

- If the function returns `nil`, the run is considered successful.
- If the function returns an error, the task is retried according to the
  retry/backoff configuration.
- The supplied `context.Context` is passed through from the caller and is
  cancelled as soon as the scheduler is stopped. Tasks should use it for
  cancellation checks.

## Behavior

### Execution Flow

1. The CRON expression is parsed once on the first `Run` call and reused.
2. The next run time is computed and awaited via a timer.
3. At the scheduled time the task is executed – with the configured
   retry/backoff logic on failure.
4. The next run time is then recomputed and the cycle repeats until the
   context is cancelled.

### Error Handling

- If an error persists after all retry attempts, it is logged and scheduling
  of the next run continues (the scheduler does not stop).
- Logging is **opt-in**. Register a callback with `WithLogging(fn)`, where
  `fn` has the signature `func(message string)`, to receive scheduler events
  (next run time, retries, failures, cancellation). Messages are already
  formatted as a single string before the callback is invoked. Without it no
  events are logged. Integrate the callback with your usual logging framework
  (e.g. `log.Println`).

## Conventions

- **Errors are returned instead of panicking** – invalid CRON expressions or
  computation errors are returned as errors.
- **No external dependencies** – the library uses only the Go standard library.
- **Efficient execution** – parsed schedules are stored as dense
  boolean lookup tables, which speeds up the hot scheduling loop.

## Acknowledgements

This project was developed with the support of artificial intelligence (AI).
The large language model **DeepSeek V4-Flash** was used to assist with writing
and reviewing parts of the codebase.
