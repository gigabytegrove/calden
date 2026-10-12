package push

import (
  "testing"
  "time"
)
func TestRetryBounds(t *testing.T){
  if retryDelay(1)<time.Second{t.Fatal("retry delay too short")}
  if retryDelay(100)>2*time.Hour{t.Fatal("retry delay unbounded")}
  if retryDelay(2)<=retryDelay(1){t.Fatal("backoff is not exponential")}
}
