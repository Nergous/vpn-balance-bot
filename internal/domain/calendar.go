package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidDate      = errors.New("invalid calendar date")
	ErrInvalidAnchorDay = errors.New("invalid billing anchor day")
	ErrNilLocation      = errors.New("location is nil")
)

// Date is a calendar day without a time of day or timezone.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// NewDate validates and creates a calendar date.
func NewDate(year int, month time.Month, day int) (Date, error) {
	if month < time.January || month > time.December {
		return Date{}, ErrInvalidDate
	}

	if day < 1 || day > daysInMonth(year, month) {
		return Date{}, ErrInvalidDate
	}

	return Date{
		Year:  year,
		Month: month,
		Day:   day,
	}, nil
}

// DateFromTime returns the calendar date of value in location.
func DateFromTime(value time.Time, location *time.Location) (Date, error) {
	if location == nil {
		return Date{}, ErrNilLocation
	}

	local := value.In(location)
	return NewDate(local.Year(), local.Month(), local.Day())
}

// ParseDate parses an ISO 8601 calendar date in YYYY-MM-DD format.
func ParseDate(value string) (Date, error) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return Date{}, fmt.Errorf("%w: %s", ErrInvalidDate, value)
	}

	return NewDate(parsed.Year(), parsed.Month(), parsed.Day())
}

// IsValid reports whether the date exists in the Gregorian calendar.
func (d Date) IsValid() bool {
	_, err := NewDate(d.Year, d.Month, d.Day)
	return err == nil
}

// String returns the ISO 8601 representation of a calendar date.
func (d Date) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

// NextBillingDate advances a billing date by one calendar month while
// preserving the original billing anchor day whenever the target month allows it.
func (d Date) NextBillingDate(anchorDay int) (Date, error) {
	if !d.IsValid() {
		return Date{}, ErrInvalidDate
	}

	if anchorDay < 1 || anchorDay > 31 {
		return Date{}, ErrInvalidAnchorDay
	}

	nextMonth := time.Date(
		d.Year,
		d.Month+1,
		1,
		0,
		0,
		0,
		0,
		time.UTC,
	)

	return NewDate(
		nextMonth.Year(),
		nextMonth.Month(),
		min(anchorDay, daysInMonth(nextMonth.Year(), nextMonth.Month())),
	)
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
