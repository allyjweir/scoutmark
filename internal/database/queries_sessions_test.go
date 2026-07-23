package database

import (
	"testing"
	"time"
)

func TestSessionDetailRowComputeStatus(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		startsAt time.Time
		endsAt   time.Time
		lockedAt *time.Time
		want     string
	}{
		{
			name:     "closed session takes precedence over lock",
			startsAt: now.Add(-2 * time.Hour),
			endsAt:   now.Add(-time.Hour),
			lockedAt: &now,
			want:     "CLOSED",
		},
		{
			name:     "active locked session",
			startsAt: now.Add(-time.Hour),
			endsAt:   now.Add(time.Hour),
			lockedAt: &now,
			want:     "LOCKED",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := SessionDetailRow{
				StartsAt: test.startsAt,
				EndsAt:   test.endsAt,
				LockedAt: test.lockedAt,
			}
			if got := session.ComputeStatus(); got != test.want {
				t.Errorf("ComputeStatus() = %q, want %q", got, test.want)
			}
		})
	}
}