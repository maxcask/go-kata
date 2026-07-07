pkg retry_backoff_policy

import (
  fmt,
  math
}

type Retryer struct {
  int MaxAttempts
  int BaseDelay
  int MaxDelay
}

isRretryable(err error) bool{

}

func (r *Retryer) Do(ctx context.Context, fn func(context.Context) error) error {
  // delay = r.BaseDelay * math.Pow(2,k)
  // cap := min(delay, MaxDelay)

  for i := range(r.MaxAttempts){
   }
}
