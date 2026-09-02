package domain

import "testing"

func TestReminderStatusIsValid(t *testing.T) {
	for _, status := range []ReminderStatus{
		ReminderStatusPending,
		ReminderStatusSent,
		ReminderStatusFailed,
		ReminderStatusSkipped,
	} {
		if !status.IsValid() {
			t.Errorf("ReminderStatus %q was rejected", status)
		}
	}

	if ReminderStatus("unknown").IsValid() {
		t.Error("unknown reminder status was accepted")
	}
}

func TestReminderTypeIsValid(t *testing.T) {
	for _, reminderType := range []ReminderType{
		ReminderTypeBeforeCharge,
		ReminderTypeChargeDebt,
		ReminderTypeOverdue3D,
		ReminderTypeOverdue7D,
		ReminderTypeManual,
	} {
		if !reminderType.IsValid() {
			t.Errorf("ReminderType %q was rejected", reminderType)
		}
	}

	if ReminderType("unknown").IsValid() {
		t.Error("unknown reminder type was accepted")
	}
}
