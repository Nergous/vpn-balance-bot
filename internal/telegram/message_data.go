package telegram

import "github.com/Nergous/vpn-balance-bot/internal/domain"

type amountMessageData struct {
	Amount   domain.AmountMinor
	Currency string
}

type statusMessageData struct {
	Name       string
	Fee        domain.AmountMinor
	Currency   string
	NextCharge domain.Date
	Balance    string
}

type adminDashboardMessageData struct {
	Total    int
	Active   int
	Paused   int
	Disabled int
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
	Fee        domain.AmountMinor
	Currency   string
	NextCharge domain.Date
}

type paymentConfirmMessageData struct {
	UserID domain.UserID
	Amount domain.AmountMinor
}
