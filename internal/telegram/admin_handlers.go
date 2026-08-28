package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
)

var ErrAdminOnly = errors.New("admin access required")

type AdminAccountService interface {
	CreateUser(context.Context, account.CreateUserParams) (domain.User, error)
	UserByID(context.Context, domain.UserID) (domain.User, error)
	ListUsers(context.Context, account.UserFilter) ([]domain.User, error)
	CreateInviteToken(context.Context, domain.UserID) (string, error)
	ChangeMonthlyFee(context.Context, account.ChangeMonthlyFeeParams) (domain.User, error)
	Pause(context.Context, domain.UserID) (domain.User, error)
	Resume(context.Context, account.ResumeParams) (domain.User, error)
	Disable(context.Context, domain.UserID) (domain.User, error)
	AddPayment(context.Context, account.AddPaymentParams) (domain.LedgerEntry, error)
	AddOpeningBalance(context.Context, account.AddOpeningBalanceParams) (domain.LedgerEntry, error)
	AddAdjustment(context.Context, account.AddAdjustmentParams) (domain.LedgerEntry, error)
	ReverseLedgerEntry(context.Context, account.ReverseLedgerEntryParams) (domain.LedgerEntry, error)
}
type AdminReminderService interface {
	DeliverManual(context.Context, domain.User, string) (bool, error)
}

type Admin struct {
	client    Client
	accounts  AdminAccountService
	adminID   int64
	wizard    *Wizard
	reminders AdminReminderService
}

func NewAdmin(client Client, accounts AdminAccountService, adminID int64, reminders ...AdminReminderService) *Admin {
	admin := &Admin{client: client, accounts: accounts, adminID: adminID, wizard: NewWizard()}
	if len(reminders) > 0 {
		admin.reminders = reminders[0]
	}
	return admin
}
func (a *Admin) authorized(userID int64) bool { return userID == a.adminID }
func (a *Admin) reject(ctx context.Context, chatID int64) error {
	_, err := a.client.SendText(ctx, chatID, "Действие доступно только администратору.")
	return err
}

func (a *Admin) CreateInvite(ctx context.Context, message IncomingMessage, userID domain.UserID) error {
	if !a.authorized(message.UserID) {
		return a.reject(ctx, message.ChatID)
	}
	token, err := a.accounts.CreateInviteToken(ctx, userID)
	if err != nil {
		return err
	}
	_, err = a.client.SendText(ctx, message.ChatID, "Invite token: "+token)
	return err
}
func (a *Admin) RemindNow(ctx context.Context, message IncomingMessage, userID domain.UserID, text string) error {
	if !a.authorized(message.UserID) {
		return a.reject(ctx, message.ChatID)
	}
	if a.reminders == nil {
		return errors.New("manual reminder service is not configured")
	}
	user, err := a.accounts.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	_, err = a.reminders.DeliverManual(ctx, user, text)
	return err
}
func (a *Admin) Dashboard(ctx context.Context, message IncomingMessage) error {
	if !a.authorized(message.UserID) {
		return a.reject(ctx, message.ChatID)
	}
	users, err := a.accounts.ListUsers(ctx, account.UserFilter{})
	if err != nil {
		return err
	}
	active, paused, disabled := 0, 0, 0
	for _, user := range users {
		switch user.Status {
		case domain.UserStatusActive:
			active++
		case domain.UserStatusPaused:
			paused++
		case domain.UserStatusDisabled:
			disabled++
		}
	}
	_, err = a.client.SendText(ctx, message.ChatID, fmt.Sprintf("Пользователи: %d\nactive: %d\npaused: %d\ndisabled: %d", len(users), active, paused, disabled))
	return err
}
func (a *Admin) Users(ctx context.Context, message IncomingMessage, filter account.UserFilter) error {
	if !a.authorized(message.UserID) {
		return a.reject(ctx, message.ChatID)
	}
	users, err := a.accounts.ListUsers(ctx, filter)
	if err != nil {
		return err
	}
	text := "Пользователей нет."
	if len(users) > 0 {
		text = ""
		for _, user := range users {
			text += fmt.Sprintf("#%d %s · %s\n", user.ID, user.DisplayName, user.Status)
		}
	}
	_, err = a.client.SendText(ctx, message.ChatID, text)
	return err
}
func (a *Admin) UserCard(ctx context.Context, message IncomingMessage, userID domain.UserID) error {
	if !a.authorized(message.UserID) {
		return a.reject(ctx, message.ChatID)
	}
	user, err := a.accounts.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	_, err = a.client.SendText(ctx, message.ChatID, fmt.Sprintf("#%d\n%s\n%s\nТариф: %d %s\nСледующее: %s", user.ID, user.DisplayName, user.Status, user.MonthlyFeeMinor, user.Currency, user.NextChargeOn))
	return err
}
func (a *Admin) CreateUser(ctx context.Context, message IncomingMessage, params account.CreateUserParams) (domain.User, error) {
	if !a.authorized(message.UserID) {
		return domain.User{}, ErrAdminOnly
	}
	return a.accounts.CreateUser(ctx, params)
}
func (a *Admin) BeginPayment(ctx context.Context, message IncomingMessage, draft PaymentDraft) error {
	if !a.authorized(message.UserID) {
		return a.reject(ctx, message.ChatID)
	}
	a.wizard.BeginPayment(message.UserID, draft)
	_, err := a.client.SendText(ctx, message.ChatID, fmt.Sprintf("Подтвердите payment: user #%d, amount %d", draft.UserID, draft.AmountMinor))
	return err
}
func (a *Admin) ConfirmPayment(ctx context.Context, message IncomingMessage) error {
	if !a.authorized(message.UserID) {
		return a.reject(ctx, message.ChatID)
	}
	return a.wizard.ConfirmPayment(ctx, message.UserID, a.accounts)
}
func (a *Admin) CancelWizard(ctx context.Context, message IncomingMessage) error {
	if !a.authorized(message.UserID) {
		return a.reject(ctx, message.ChatID)
	}
	a.wizard.Cancel(message.UserID)
	_, err := a.client.SendText(ctx, message.ChatID, "Операция отменена.")
	return err
}
func (a *Admin) Pause(ctx context.Context, message IncomingMessage, userID domain.UserID) error {
	if !a.authorized(message.UserID) {
		return a.reject(ctx, message.ChatID)
	}
	_, err := a.accounts.Pause(ctx, userID)
	return err
}
func (a *Admin) Disable(ctx context.Context, message IncomingMessage, userID domain.UserID) error {
	if !a.authorized(message.UserID) {
		return a.reject(ctx, message.ChatID)
	}
	_, err := a.accounts.Disable(ctx, userID)
	return err
}
func (a *Admin) Resume(ctx context.Context, message IncomingMessage, params account.ResumeParams) error {
	if !a.authorized(message.UserID) {
		return a.reject(ctx, message.ChatID)
	}
	_, err := a.accounts.Resume(ctx, params)
	return err
}
func (a *Admin) ChangeFee(ctx context.Context, message IncomingMessage, params account.ChangeMonthlyFeeParams) error {
	if !a.authorized(message.UserID) {
		return a.reject(ctx, message.ChatID)
	}
	_, err := a.accounts.ChangeMonthlyFee(ctx, params)
	return err
}
func (a *Admin) AddOpeningBalance(ctx context.Context, message IncomingMessage, params account.AddOpeningBalanceParams) error {
	if !a.authorized(message.UserID) {
		return a.reject(ctx, message.ChatID)
	}
	params.AdminTelegramID = message.UserID
	_, err := a.accounts.AddOpeningBalance(ctx, params)
	return err
}
func (a *Admin) AddAdjustment(ctx context.Context, message IncomingMessage, params account.AddAdjustmentParams) error {
	if !a.authorized(message.UserID) {
		return a.reject(ctx, message.ChatID)
	}
	params.AdminTelegramID = message.UserID
	_, err := a.accounts.AddAdjustment(ctx, params)
	return err
}
func (a *Admin) Reverse(ctx context.Context, message IncomingMessage, params account.ReverseLedgerEntryParams) error {
	if !a.authorized(message.UserID) {
		return a.reject(ctx, message.ChatID)
	}
	params.AdminTelegramID = message.UserID
	_, err := a.accounts.ReverseLedgerEntry(ctx, params)
	return err
}

// HandleAdminCommand is the production command boundary for admin scenarios.
// Supported forms: /admin; /admin users [status]; /admin invite <user-id>;
// /admin payment <user-id> <amount> [note]; /admin confirm; /admin cancel.
func (b *Bot) HandleAdminCommand(ctx context.Context, message IncomingMessage) error {
	if b.admin == nil {
		return b.send(ctx, message.ChatID, "Админ-панель не настроена.")
	}
	if !b.admin.authorized(message.UserID) {
		return b.admin.reject(ctx, message.ChatID)
	}
	parts := strings.Fields(message.Text)
	if len(parts) <= 1 {
		return b.admin.Dashboard(ctx, message)
	}
	switch parts[1] {
	case "users":
		var filter account.UserFilter
		if len(parts) == 3 {
			status := domain.UserStatus(parts[2])
			if !status.IsValid() {
				return b.send(ctx, message.ChatID, "Статус: active, paused или disabled.")
			}
			filter.Status = &status
		}
		return b.admin.Users(ctx, message, filter)
	case "invite":
		userID, err := adminUserID(parts, 2)
		if err != nil {
			return b.send(ctx, message.ChatID, err.Error())
		}
		return b.admin.CreateInvite(ctx, message, userID)
	case "payment":
		if len(parts) < 4 {
			return b.send(ctx, message.ChatID, "Используйте: /admin payment <user-id> <amount> [note]")
		}
		userID, err := adminUserID(parts, 2)
		if err != nil {
			return b.send(ctx, message.ChatID, err.Error())
		}
		amount, err := strconv.ParseInt(parts[3], 10, 64)
		if err != nil {
			return b.send(ctx, message.ChatID, "Сумма должна быть целым числом копеек.")
		}
		var note *string
		if value := strings.TrimSpace(strings.Join(parts[4:], " ")); value != "" {
			note = &value
		}
		return b.admin.BeginPayment(ctx, message, PaymentDraft{UserID: userID, AmountMinor: domain.AmountMinor(amount), Note: note})
	case "confirm":
		return b.admin.ConfirmPayment(ctx, message)
	case "cancel":
		return b.admin.CancelWizard(ctx, message)
	case "remind":
		userID, err := adminUserID(parts, 2)
		if err != nil {
			return b.send(ctx, message.ChatID, err.Error())
		}
		text := strings.TrimSpace(strings.Join(parts[3:], " "))
		if text == "" {
			return b.send(ctx, message.ChatID, "Используйте: /admin remind <user-id> <text>")
		}
		return b.admin.RemindNow(ctx, message, userID, text)
	default:
		return b.send(ctx, message.ChatID, "Команды: /admin, /admin users [status], /admin invite <id>, /admin payment <id> <amount> [note], /admin confirm, /admin cancel")
	}
}

func adminUserID(parts []string, index int) (domain.UserID, error) {
	if len(parts) <= index {
		return 0, errors.New("Не указан user ID.")
	}
	value, err := strconv.ParseInt(parts[index], 10, 64)
	if err != nil || value <= 0 {
		return 0, errors.New("User ID должен быть положительным числом.")
	}
	return domain.UserID(value), nil
}
