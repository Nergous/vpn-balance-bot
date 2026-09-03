package telegram

import "github.com/Nergous/vpn-balance-bot/internal/domain"

type amountMessageData struct {
	Amount         string
	Currency       string
	CoveredPeriods int64
}

type statusMessageData struct {
	Name       string
	Fee        string
	Currency   string
	NextCharge domain.Date
	Balance    string
}

type adminDashboardMessageData struct {
	Total        int
	Active       int
	Paused       int
	Disabled     int
	Debtors      int
	Insufficient int
	Unlinked     int
	Unreachable  int
}

type userLineMessageData struct {
	ID     domain.UserID
	Name   string
	Status domain.UserStatus
}

type userCardMessageData struct {
	ID         domain.UserID
	Name       string
	Status     domain.UserStatus
	Fee        string
	Currency   string
	NextCharge domain.Date
}

type paymentConfirmMessageData struct {
	UserID         domain.UserID
	UserName       string
	Amount         string
	Currency       string
	OccurredAt     string
	Note           string
	ConfirmCommand string
	CancelCommand  string
}
