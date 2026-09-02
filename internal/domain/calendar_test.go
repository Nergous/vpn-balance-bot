package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewDate(t *testing.T) {
	tests := []struct {
		name  string
		year  int
		month time.Month
		day   int
		err   error
	}{
		{
			name:  "valid leap day",
			year:  2024,
			month: time.February,
			day:   29,
		},
		{
			name:  "invalid non leap day",
			year:  2025,
			month: time.February,
			day:   29,
			err:   ErrInvalidDate,
		},
		{
			name:  "invalid month",
			year:  2025,
			month: 13,
			day:   1,
			err:   ErrInvalidDate,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NewDate(test.year, test.month, test.day)
			if !errors.Is(err, test.err) {
				t.Fatalf("NewDate() error = %v, want %v", err, test.err)
			}
			if err == nil && !got.IsValid() {
				t.Fatalf("NewDate() returned invalid date: %+v", got)
			}
		})
	}
}

func TestDateNextBillingDate(t *testing.T) {
	tests := []struct {
		name      string
		current   Date
		anchorDay int
		want      Date
	}{
		{
			name:      "January 31 to February 28",
			current:   Date{Year: 2025, Month: time.January, Day: 31},
			anchorDay: 31,
			want:      Date{Year: 2025, Month: time.February, Day: 28},
		},
		{
			name:      "leap year January 31 to February 29",
			current:   Date{Year: 2024, Month: time.January, Day: 31},
			anchorDay: 31,
			want:      Date{Year: 2024, Month: time.February, Day: 29},
		},
		{
			name:      "February 28 restores anchor day 31",
			current:   Date{Year: 2025, Month: time.February, Day: 28},
			anchorDay: 31,
			want:      Date{Year: 2025, Month: time.March, Day: 31},
		},
		{
			name:      "January 30 to February 28",
			current:   Date{Year: 2025, Month: time.January, Day: 30},
			anchorDay: 30,
			want:      Date{Year: 2025, Month: time.February, Day: 28},
		},
		{
			name:      "February 28 restores anchor day 30",
			current:   Date{Year: 2025, Month: time.February, Day: 28},
			anchorDay: 30,
			want:      Date{Year: 2025, Month: time.March, Day: 30},
		},
		{
			name:      "March 31 to April 30",
			current:   Date{Year: 2025, Month: time.March, Day: 31},
			anchorDay: 31,
			want:      Date{Year: 2025, Month: time.April, Day: 30},
		},
		{
			name:      "February 29 to March 29",
			current:   Date{Year: 2024, Month: time.February, Day: 29},
			anchorDay: 29,
			want:      Date{Year: 2024, Month: time.March, Day: 29},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.current.NextBillingDate(test.anchorDay)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("NextBillingDate() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestDateNextBillingDateRejectsInvalidInput(t *testing.T) {
	date := Date{Year: 2025, Month: time.January, Day: 31}

	for _, anchorDay := range []int{0, 32} {
		_, err := date.NextBillingDate(anchorDay)
		if !errors.Is(err, ErrInvalidAnchorDay) {
			t.Errorf("anchor day %d error = %v, want %v", anchorDay, err, ErrInvalidAnchorDay)
		}
	}

	_, err := (Date{Year: 2025, Month: time.February, Day: 29}).NextBillingDate(29)
	if !errors.Is(err, ErrInvalidDate) {
		t.Fatalf("invalid current date error = %v, want %v", err, ErrInvalidDate)
	}
}

func TestDateFromTime(t *testing.T) {
	location := time.FixedZone("UTC+3", 3*60*60)
	value := time.Date(2025, time.December, 31, 22, 30, 0, 0, time.UTC)

	got, err := DateFromTime(value, location)
	if err != nil {
		t.Fatal(err)
	}

	want := Date{Year: 2026, Month: time.January, Day: 1}
	if got != want {
		t.Fatalf("DateFromTime() = %+v, want %+v", got, want)
	}

	if _, err := DateFromTime(value, nil); !errors.Is(err, ErrNilLocation) {
		t.Fatalf("DateFromTime(nil location) error = %v, want %v", err, ErrNilLocation)
	}
}

func TestParseDateAndString(t *testing.T) {
	date, err := ParseDate("2025-08-27")
	if err != nil {
		t.Fatal(err)
	}
	if got := date.String(); got != "2025-08-27" {
		t.Fatalf("String() = %q, want 2025-08-27", got)
	}

	if _, err := ParseDate("27-08-2025"); !errors.Is(err, ErrInvalidDate) {
		t.Fatalf("ParseDate() error = %v, want %v", err, ErrInvalidDate)
	}
}
