package reminder

import (
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

func TestSelectAutomatic(t *testing.T) {
	next, _ := domain.NewDate(2026, time.September, 10)
	user := domain.User{Status: domain.UserStatusActive, MonthlyFeeMinor: 100, NextChargeOn: next}
	cases := []struct {
		name    string
		today   domain.Date
		balance domain.AmountMinor
		want    domain.ReminderType
	}{
		{"before", mustDate(t, 2026, time.September, 7), 0, domain.ReminderTypeBeforeCharge},
		{"charge debt", next, -1, domain.ReminderTypeChargeDebt},
		{"overdue 3", mustDate(t, 2026, time.September, 13), -1, domain.ReminderTypeOverdue3D},
		{"overdue 7 priority", mustDate(t, 2026, time.September, 17), -1, domain.ReminderTypeOverdue7D},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := SelectAutomatic(user, tc.balance, tc.today)
			if !ok || got != tc.want {
				t.Fatalf("got %q,%t want %q", got, ok, tc.want)
			}
		})
	}
	if _, ok := SelectAutomatic(user, 100, mustDate(t, 2026, time.September, 7)); ok {
		t.Fatal("prepayment or coverage produced reminder")
	}
	user.Status = domain.UserStatusPaused
	if _, ok := SelectAutomatic(user, -1, mustDate(t, 2026, time.September, 17)); ok {
		t.Fatal("paused produced reminder")
	}
}
func mustDate(t *testing.T, y int, m time.Month, d int) domain.Date {
	t.Helper()
	date, err := domain.NewDate(y, m, d)
	if err != nil {
		t.Fatal(err)
	}
	return date
}
