package reminder

import "context"

// Sender is a narrow transport boundary for reminder delivery.
type Sender interface {
	SendReminder(context.Context, int64, string) (int, error)
}
