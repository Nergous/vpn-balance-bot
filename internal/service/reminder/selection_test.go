package reminder

import (
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

func TestSelectAutomatic(t *testing.T) {
	next := mustDate(t, 2026, time.October, 1)
	charged := mustDate(t, 2026, time.September, 1)
	user := domain.User{Status: domain.UserStatusActive, MonthlyFeeMinor: 100, NextChargeOn: next}
	cases := []struct {
		name       string
		today      domain.Date
		balance    domain.AmountMinor
		latest     *domain.Date
		wantType   domain.ReminderType
		wantPeriod domain.Date
	}{
		{"before uses next charge", mustDate(t, 2026, time.September, 28), 0, &charged, domain.ReminderTypeBeforeCharge, next},
		{"charge debt uses charged period", charged, -1, &charged, domain.ReminderTypeChargeDebt, charged},
		{"overdue 3 uses charged period", mustDate(t, 2026, time.September, 4), -1, &charged, domain.ReminderTypeOverdue3D, charged},
		{"overdue 7 uses charged period", mustDate(t, 2026, time.September, 8), -1, &charged, domain.ReminderTypeOverdue7D, charged},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotType, gotPeriod, ok := SelectAutomatic(Candidate{
				User:                 user,
				Balance:              tc.balance,
				LatestChargePeriodOn: tc.latest,
			}, tc.today)
			if !ok || gotType != tc.wantType || gotPeriod != tc.wantPeriod {
				t.Fatalf("got %q,%s,%t want %q,%s", gotType, gotPeriod, ok, tc.wantType, tc.wantPeriod)
			}
		})
	}
	if _, _, ok := SelectAutomatic(Candidate{User: user, Balance: 100, LatestChargePeriodOn: &charged}, charged); ok {
		t.Fatal("prepayment or coverage produced reminder")
	}
	user.Status = domain.UserStatusPaused
	if _, _, ok := SelectAutomatic(Candidate{User: user, Balance: -1, LatestChargePeriodOn: &charged}, charged); ok {
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
