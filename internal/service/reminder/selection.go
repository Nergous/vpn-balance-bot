package reminder

import (
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

// SelectAutomatic chooses the single highest-priority reminder applicable today.
func SelectAutomatic(user domain.User, balance domain.AmountMinor, today domain.Date) (domain.ReminderType, bool) {
	if user.Status != domain.UserStatusActive || balance >= user.MonthlyFeeMinor {
		return "", false
	}

	next := user.NextChargeOn
	if today == next && balance < 0 {
		return domain.ReminderTypeChargeDebt, true
	}
	if isDaysBefore(today, next, 3) && balance < user.MonthlyFeeMinor {
		return domain.ReminderTypeBeforeCharge, true
	}
	if balance < 0 && isDaysAfter(today, next, 7) {
		return domain.ReminderTypeOverdue7D, true
	}
	if balance < 0 && isDaysAfter(today, next, 3) {
		return domain.ReminderTypeOverdue3D, true
	}
	return "", false
}

func isDaysBefore(today, target domain.Date, days int) bool {
	return dateTime(target).Sub(dateTime(today)) == time.Duration(days)*24*time.Hour
}
func isDaysAfter(today, target domain.Date, days int) bool {
	return dateTime(today).Sub(dateTime(target)) >= time.Duration(days)*24*time.Hour
}
func dateTime(date domain.Date) time.Time {
	return time.Date(date.Year, date.Month, date.Day, 0, 0, 0, 0, time.UTC)
}
