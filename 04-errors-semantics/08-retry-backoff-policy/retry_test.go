package retry_backoff_policy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryerDo_SuccessOnFirstAttempt(t *testing.T) {
	r := &Retryer{MaxAttempts: 3}
	calls := 0

	err := r.Do(context.Background(), func(context.Context) error {
		calls++
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestRetryerDo_NonRetryableFailsWithoutRetry(t *testing.T) {
	r := &Retryer{MaxAttempts: 5}
	calls := 0
	permErr := errors.New("permanent failure")

	err := r.Do(context.Background(), func(context.Context) error {
		calls++
		return permErr
	})

	require.Error(t, err)
	assert.Equal(t, 1, calls)
	assert.ErrorContains(t, err, "attempt 1 failed")
	assert.ErrorIs(t, err, permErr)
}

func TestRetryerDo_TransientThenSuccess_RetriesUntilSuccess(t *testing.T) {
	r := &Retryer{MaxAttempts: 5}
	calls := 0

	err := r.Do(context.Background(), func(context.Context) error {
		calls++
		if calls < 3 {
			return fmt.Errorf("temporary outage: %w", ErrTransient)
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 3, calls, "must retry transient failures and stop on first success")
}

func TestRetryerDo_TransientExhaustsMaxAttempts(t *testing.T) {
	r := &Retryer{MaxAttempts: 3}
	calls := 0
	rootErr := fmt.Errorf("db throttle: %w", ErrTransient)

	err := r.Do(context.Background(), func(context.Context) error {
		calls++
		return rootErr
	})

	require.Error(t, err)
	assert.Equal(t, 3, calls, "must stop exactly at MaxAttempts")
	assert.ErrorContains(t, err, "all 3 attempts failed")
	assert.ErrorIs(t, err, rootErr)
	assert.ErrorIs(t, err, ErrTransient)
}

func TestRetryerDo_ContextCanceledDuringBackoff(t *testing.T) {
	r := &Retryer{
		MaxAttempts: 3,
		BaseDelay:   time.Hour,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := 0
	err := r.Do(ctx, func(context.Context) error {
		calls++
		if calls == 1 {
			cancel()
		}
		return fmt.Errorf("please retry: %w", ErrTransient)
	})

	require.Error(t, err)
	assert.Equal(t, 1, calls, "must stop immediately after context cancellation")
	assert.ErrorContains(t, err, "retry canceled after attempt 1")
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRetryer_BackoffDelay_ExponentialWithCap(t *testing.T) {
	r := &Retryer{
		BaseDelay: 100 * time.Millisecond,
		MaxDelay:  350 * time.Millisecond,
	}

	assert.Equal(t, 100*time.Millisecond, r.backoffDelay(0))
	assert.Equal(t, 200*time.Millisecond, r.backoffDelay(1))
	assert.Equal(t, 350*time.Millisecond, r.backoffDelay(2), "400ms should be capped to 350ms")
	assert.Equal(t, 350*time.Millisecond, r.backoffDelay(5), "must stay capped on later attempts")
}

type timeoutNetErr struct{}

func (timeoutNetErr) Error() string   { return "network timeout" }
func (timeoutNetErr) Timeout() bool   { return true }
func (timeoutNetErr) Temporary() bool { return false }

var _ net.Error = timeoutNetErr{}

func TestRetryerDo_NetTimeoutError_IsRetryable(t *testing.T) {
	r := &Retryer{MaxAttempts: 3}
	calls := 0

	err := r.Do(context.Background(), func(context.Context) error {
		calls++
		if calls < 3 {
			return timeoutNetErr{}
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 3, calls, "net timeout errors should be retried")
}
