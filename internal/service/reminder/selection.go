package reminder

import (
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

// SelectAutomatic chooses the single highest-priority reminder applicable today
// and the billing period used for delivery deduplication.
func SelectAutomatic(candidate Candidate, today domain.Date) (domain.ReminderType, domain.Date, bool) {
	user := candidate.User
	if user.Status != domain.UserStatusActive || candidate.Balance >= user.MonthlyFeeMinor {
		return "", domain.Date{}, false
	}

	if candidate.LatestChargePeriodOn != nil && today == *candidate.LatestChargePeriodOn && candidate.Balance < 0 {
		return domain.ReminderTypeChargeDebt, *candidate.LatestChargePeriodOn, true
	}

	if isDaysBefore(today, user.NextChargeOn, 3) {
		return domain.ReminderTypeBeforeCharge, user.NextChargeOn, true
	}

	if candidate.LatestChargePeriodOn != nil && candidate.Balance < 0 && isDaysAfter(today, *candidate.LatestChargePeriodOn, 7) {
		return domain.ReminderTypeOverdue7D, *candidate.LatestChargePeriodOn, true
	}

	if candidate.LatestChargePeriodOn != nil && candidate.Balance < 0 && isDaysAfter(today, *candidate.LatestChargePeriodOn, 3) {
		return domain.ReminderTypeOverdue3D, *candidate.LatestChargePeriodOn, true
	}

	return "", domain.Date{}, false
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
