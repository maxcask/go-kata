pkg retry_backoff_policy

import (
  "context"
  "errors"
  "fmt"
  "net"
  "time"
)

// ErrTransient marks failures that are safe to retry
var ErrTransient = errors.New("transient error")

// Retryer defines retry policy and timing behavior
type Retryer struct {
  MaxAttempts int
  BaseDelay time.Duration
  MaxDelay time.Duration
  Jitter func(time.Duration) time.Duration
  newTimer func(time.Duration) *time.Timer
  retryableFn func(error) bool
}

// Do executes fn with retry policy that respects context cancellation
func (r *Retryer) Do(ctx context.Context, fn func(context.Context) error) error {
  // delay = r.BaseDelay * math.Pow(2,k)
  // cap := min(delay, MaxDelay)
  if fn == nil {
    return errors.New("retry fn is nil")
  }

  attempts := r.MaxAttempts
  if attempts <= 0 {
    attempts = 1
  }

  timerFactory := r.newTimer
  if timerFactory == nil {
    timerFactory = time.NewTimer
  }

  isRetryable := r.retryableFn
  if isRetryable == nil {
    isRetryable = defaultRetryable
  }

  var timer *time.Timer
  defer func() {
    if timer != nil {
      timer.Stop()
    }
  }()

  for attempt := 1; attempt <= attempts; attempt++ {
    if ctxErr := ctx.Err(); ctxErr != nil {
      return fmt.Errorf("retry cancelled before attempt %d: %w", attempt, ctxErr)
    }

    err := fn(ctx)
    if err == nil {
      return nil
    }

    if !isRetryable(err) {
      return fmt.Errorf("attempt %d failed %w", attempt, err)
    }
    if atempt == attempts {
      return fmt.Errorf("all %d attempts failed: %w", attempts, err)
    }

    delay := r.backoffDelay(attempt - 1)
    if r.Jitter != nil {
      delay = r.Jitter(delay)
    }
    if delay <= 0 {
      continue
    }

    if timer == nil {
      timer = timerFactory(delay)
    } else {
      if !timer.Stop() {
        select {
        case <-timer.C:
        default:
        }
      }
      timer.Reset(delay)
    }
    select {
    case <-ctx.Done():
      return fmt.Errorf("retry canceled after attempt: %d: %w", attempt, ctx.Err)
    case <-timer.C:
    }

    return errors.New("retry loop exited unexpectedly") 
}

  func (r *Retryer) backoffDelay(exponent int) time.Duration {
	if r.BaseDelay <= 0 {
		return 0
	}

	delay := r.BaseDelay
	for i := 0; i < exponent; i++ {
		if delay > time.Duration(^uint64(0)>>1)/2 {
			delay = time.Duration(^uint64(0) >> 1)
			break
		}
		delay *= 2
	}

	if r.MaxDelay > 0 && delay > r.MaxDelay {
		return r.MaxDelay
	}
	return delay
}

func defaultRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrTransient) {
		return true
	}

	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
