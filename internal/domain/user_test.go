package domain

import "testing"

func TestUserStatusIsValid(t *testing.T) {
	for _, status := range []UserStatus{
		UserStatusActive,
		UserStatusPaused,
		UserStatusDisabled,
	} {
		if !status.IsValid() {
			t.Errorf("UserStatus %q was rejected", status)
		}
	}

	if UserStatus("unknown").IsValid() {
		t.Error("unknown user status was accepted")
	}
}

func TestUserStatusCanTransitionTo(t *testing.T) {
	tests := []struct {
		from UserStatus
		to   UserStatus
		want bool
	}{
		{from: UserStatusActive, to: UserStatusPaused, want: true},
		{from: UserStatusActive, to: UserStatusDisabled, want: true},
		{from: UserStatusPaused, to: UserStatusActive, want: true},
		{from: UserStatusPaused, to: UserStatusDisabled, want: true},
		{from: UserStatusActive, to: UserStatusActive},
		{from: UserStatusPaused, to: UserStatusPaused},
		{from: UserStatusDisabled, to: UserStatusActive},
		{from: UserStatusDisabled, to: UserStatusPaused},
		{from: UserStatusDisabled, to: UserStatusDisabled},
	}

	for _, test := range tests {
		if got := test.from.CanTransitionTo(test.to); got != test.want {
			t.Errorf("%q.CanTransitionTo(%q) = %t, want %t", test.from, test.to, got, test.want)
		}
	}
}
