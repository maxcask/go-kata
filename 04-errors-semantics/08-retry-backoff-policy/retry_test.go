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

func TestRetryerDo_Table(t *testing.T) {
	errPermanent := errors.New("permanent failure")
	errTransientWrapped := fmt.Errorf("db throttle: %w", ErrTransient)

	type testCase struct {
		name        string
		retryer     Retryer
		buildFn     func(t *testing.T, calls *int) (context.Context, func(context.Context) error)
		wantErr     bool
		wantCalls   int
		errContains []string
		errIs       []error
	}

	tests := []testCase{
		{
			name:    "success on first attempt",
			retryer: Retryer{MaxAttempts: 3},
			buildFn: func(_ *testing.T, calls *int) (context.Context, func(context.Context) error) {
				return context.Background(), func(context.Context) error {
					*calls++
					return nil
				}
			},
			wantErr:   false,
			wantCalls: 1,
		},
		{
			name:    "non-retryable fails without retry",
			retryer: Retryer{MaxAttempts: 5},
			buildFn: func(_ *testing.T, calls *int) (context.Context, func(context.Context) error) {
				return context.Background(), func(context.Context) error {
					*calls++
					return errPermanent
				}
			},
			wantErr:     true,
			wantCalls:   1,
			errContains: []string{"attempt 1 failed"},
			errIs:       []error{errPermanent},
		},
		{
			name:    "transient then success",
			retryer: Retryer{MaxAttempts: 5},
			buildFn: func(_ *testing.T, calls *int) (context.Context, func(context.Context) error) {
				return context.Background(), func(context.Context) error {
					*calls++
					if *calls < 3 {
						return fmt.Errorf("temporary outage: %w", ErrTransient)
					}
					return nil
				}
			},
			wantErr:   false,
			wantCalls: 3,
		},
		{
			name:    "transient exhausts max attempts",
			retryer: Retryer{MaxAttempts: 3},
			buildFn: func(_ *testing.T, calls *int) (context.Context, func(context.Context) error) {
				return context.Background(), func(context.Context) error {
					*calls++
					return errTransientWrapped
				}
			},
			wantErr:     true,
			wantCalls:   3,
			errContains: []string{"all 3 attempts failed"},
			errIs:       []error{errTransientWrapped, ErrTransient},
		},
		{
			name: "context canceled during backoff",
			retryer: Retryer{
				MaxAttempts: 3,
				BaseDelay:   time.Hour,
			},
			buildFn: func(t *testing.T, calls *int) (context.Context, func(context.Context) error) {
				ctx, cancel := context.WithCancel(context.Background())
				t.Cleanup(cancel)
				return ctx, func(context.Context) error {
					*calls++
					if *calls == 1 {
						cancel()
					}
					return fmt.Errorf("please retry: %w", ErrTransient)
				}
			},
			wantErr:     true,
			wantCalls:   1,
			errContains: []string{"retry canceled after attempt 1"},
			errIs:       []error{context.Canceled},
		},
		{
			name:    "net timeout is retryable",
			retryer: Retryer{MaxAttempts: 3},
			buildFn: func(_ *testing.T, calls *int) (context.Context, func(context.Context) error) {
				return context.Background(), func(context.Context) error {
					*calls++
					if *calls < 3 {
						return timeoutNetErr{}
					}
					return nil
				}
			},
			wantErr:   false,
			wantCalls: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			ctx, fn := tt.buildFn(t, &calls)

			err := tt.retryer.Do(ctx, fn)

			if tt.wantErr {
				require.Error(t, err)
				for _, wantContains := range tt.errContains {
					assert.ErrorContains(t, err, wantContains)
				}
				for _, wantErrIs := range tt.errIs {
					assert.ErrorIs(t, err, wantErrIs)
				}
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantCalls, calls)
		})
	}
}

func TestRetryer_BackoffDelay_Table(t *testing.T) {
	r := &Retryer{
		BaseDelay: 100 * time.Millisecond,
		MaxDelay:  350 * time.Millisecond,
	}

	tests := []struct {
		name     string
		exponent int
		want     time.Duration
	}{
		{name: "base delay", exponent: 0, want: 100 * time.Millisecond},
		{name: "double once", exponent: 1, want: 200 * time.Millisecond},
		{name: "cap reached", exponent: 2, want: 350 * time.Millisecond},
		{name: "cap stays", exponent: 5, want: 350 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, r.backoffDelay(tt.exponent))
		})
	}
}

type timeoutNetErr struct{}

func (timeoutNetErr) Error() string   { return "network timeout" }
func (timeoutNetErr) Timeout() bool   { return true }
func (timeoutNetErr) Temporary() bool { return false }

var _ net.Error = timeoutNetErr{}
