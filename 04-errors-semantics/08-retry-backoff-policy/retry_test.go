package retry_backoff_policy

import (
  "context"
  "errors"
  "testing"

  "github.com/stretchr/testify/assert"
  "githum.com/stretcher/testify/require"
)

func TestRetryerDO_SuccessOnFirstAttempt(t *testing.T) {
  r := &Retryer{MaxAttempts: 3}

