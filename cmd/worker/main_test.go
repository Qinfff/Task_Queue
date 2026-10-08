package main

import (
	"testing"
	"time"
)

func TestBackoff(t *testing.T) {

	tests := []struct{
		retries int
		want time.Duration
	}{
		{0,1*time.Second},
		{1,2*time.Second},
		{6,64*time.Second},
		{100,64*time.Second},
	}

	for _, tt := range tests{
		got := backoff(tt.retries)
		if tt.want != got{
			t.Errorf("backoff(%d) = %v, 期望 %v", tt.retries,got, tt.want)
		}
	}
}