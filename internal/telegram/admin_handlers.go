package telegram

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/localization"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
)

// ErrAdminOnly is returned when a non-admin attempts a state-changing operation.
var ErrAdminOnly = errors.New("admin access required")

type adminAcknowledgementError struct {
	cause error
}

func (e adminAcknowledgementError) Error() string { return "send administrator acknowledgement" }
func (e adminAcknowledgementError) Unwrap() error { return e.cause }

const adminUserPageSize = 50

// AdminAccountService contains administrator-only account and ledger operations.
type AdminAccountService interface {
	CreateUser(context.Context, account.CreateUserParams) (domain.User, error)
	UserByID(context.Context, domain.UserID) (domain.User, error)
	ListUsers(context.Context, account.UserFilter) ([]domain.User, error)
	CreateInviteToken(context.Context, account.CreateInviteTokenParams) (string, error)
	ChangeMonthlyFee(context.Context, account.ChangeMonthlyFeeParams) (domain.User, error)
	Pause(context.Context, account.AdminUserParams) (domain.User, error)
	Resume(context.Context, account.ResumeParams) (domain.User, error)
	Disable(context.Context, account.AdminUserParams) (domain.User, error)
	AddPayment(context.Context, account.AddPaymentParams) (domain.LedgerEntry, error)
	Balance(context.Context, domain.UserID) (domain.AmountMinor, error)
	AddOpeningBalance(context.Context, account.AddOpeningBalanceParams) (domain.LedgerEntry, error)
	AddAdjustment(context.Context, account.AddAdjustmentParams) (domain.LedgerEntry, error)
	ReverseLedgerEntry(context.Context, account.ReverseLedgerEntryParams) (domain.LedgerEntry, error)
}

type adminQueryService interface {
	ListUsersPage(context.Context, account.UserFilter, domain.UserID, int) ([]domain.User, bool, error)
	UserStatusCounts(context.Context) (account.UserStatusCounts, error)
}

// AdminReminderService delivers an explicit reminder through the shared delivery flow.
type AdminReminderService interface {
	DeliverManual(context.Context, domain.User, int64, string) (bool, error)
	ConfirmUnknownNotSent(context.Context, domain.UserID, domain.Date, domain.ReminderType, string) (bool, error)
}

// Admin adapts authenticated administrator commands to service use cases.
type Admin struct {
	client    Client
	accounts  AdminAccountService
	adminID   int64
	wizard    *Wizard
	reminders AdminReminderService
	language  localization.Language
	now       func() time.Time
}

// NewAdmin creates an admin handler for one numeric Telegram administrator ID.
func NewAdmin(
	client Client,
	accounts AdminAccountService,
	reminders AdminReminderService,
	adminID int64,
	language localization.Language,
) *Admin {
	localization.MustNew(language)
	return &Admin{
		client:    client,
		accounts:  accounts,
		reminders: reminders,
		adminID:   adminID,
		wizard:    NewWizard(paymentDraftStore(accounts)),
		language:  language,
		now:       time.Now,
	}
}

func (a *Admin) authorized(message IncomingMessage) bool {
	return message.isPrivate() && message.UserID == a.adminID
}

func (a *Admin) reject(ctx context.Context, chatID int64) error {
	_, err := a.client.SendText(ctx, chatID, localized(a.language, "AdminDenied"))
	return err
}

func (a *Admin) CreateInvite(ctx context.Context, message IncomingMessage, userID domain.UserID) error {
	if !a.authorized(message) {
		return a.reject(ctx, message.ChatID)
	}

	token, err := a.accounts.CreateInviteToken(ctx, account.CreateInviteTokenParams{
		AdminTelegramID: message.UserID,
		UserID:          userID,
	})
	if err != nil {
		return err
	}

	_, err = a.client.SendInviteToken(
		ctx,
		message.ChatID,
		localized(a.language, "InviteTokenTitle"),
		localized(a.language, "CopyInviteToken"),
		token,
	)
	return err
}

// RemindNow sends an administrator-requested reminder through reminder delivery storage.
func (a *Admin) RemindNow(ctx context.Context, message IncomingMessage, userID domain.UserID, text string) error {
	if !a.authorized(message) {
		return a.reject(ctx, message.ChatID)
	}

	user, err := a.accounts.UserByID(ctx, userID)
	if err != nil {
		return err
	}

	delivered, err := a.reminders.DeliverManual(ctx, user, message.UpdateID, text)
	if err != nil {
		return err
	}
	key := "AdminReminderSkipped"
	if delivered {
		key = "AdminReminderSent"
	}
	_, err = a.client.SendText(ctx, message.ChatID, localized(a.language, key))
	if err != nil {
		return adminAcknowledgementError{cause: err}
	}
	return nil
}

// ReconcileReminder reopens an ambiguous reminder only after the administrator
// has verified that Telegram did not deliver the original message.
func (a *Admin) ReconcileReminder(ctx context.Context, message IncomingMessage, userID domain.UserID, billingDate domain.Date, reminderType domain.ReminderType, deliveryKey string) error {
	if !a.authorized(message) {
		return a.reject(ctx, message.ChatID)
	}

	marked, err := a.reminders.ConfirmUnknownNotSent(ctx, userID, billingDate, reminderType, deliveryKey)
	if err != nil {
		return err
	}
	key := "AdminReconcileMissing"
	if marked {
		key = "AdminReconcileReady"
	}
	_, err = a.client.SendText(ctx, message.ChatID, localized(a.language, key))
	return err
}

func (a *Admin) Dashboard(ctx context.Context, message IncomingMessage) error {
	if !a.authorized(message) {
		return a.reject(ctx, message.ChatID)
	}

	var counts account.UserStatusCounts
	if queries, ok := a.accounts.(adminQueryService); ok {
		var err error
		counts, err = queries.UserStatusCounts(ctx)
		if err != nil {
			return err
		}
	} else {
		users, err := a.accounts.ListUsers(ctx, account.UserFilter{})
		if err != nil {
			return err
		}
		counts.Total = len(users)
		for _, user := range users {
			switch user.Status {
			case domain.UserStatusActive:
				counts.Active++
			case domain.UserStatusPaused:
				counts.Paused++
			case domain.UserStatusDisabled:
				counts.Disabled++
			}
			balance, balanceErr := a.accounts.Balance(ctx, user.ID)
			if balanceErr != nil {
				return balanceErr
			}
			if balance < 0 {
				counts.Debtors++
			}
			if user.Status == domain.UserStatusActive && balance < user.MonthlyFeeMinor {
				counts.Insufficient++
			}
			if user.TelegramUserID == nil || user.TelegramChatID == nil {
				counts.Unlinked++
			}
		}
	}

	text := localized(a.language, "AdminDashboard", adminDashboardMessageData{
		Total:        counts.Total,
		Active:       counts.Active,
		Paused:       counts.Paused,
		Disabled:     counts.Disabled,
		Debtors:      counts.Debtors,
		Insufficient: counts.Insufficient,
		Unlinked:     counts.Unlinked,
		Unreachable:  counts.Unreachable,
	})

	_, err := a.client.SendText(ctx, message.ChatID, text)
	return err
}

func (a *Admin) Users(ctx context.Context, message IncomingMessage, filter account.UserFilter, afterID domain.UserID) error {
	if !a.authorized(message) {
		return a.reject(ctx, message.ChatID)
	}

	var users []domain.User
	var hasMore bool
	if queries, ok := a.accounts.(adminQueryService); ok {
		var err error
		users, hasMore, err = queries.ListUsersPage(ctx, filter, afterID, adminUserPageSize)
		if err != nil {
			return err
		}
	} else {
		all, err := a.accounts.ListUsers(ctx, filter)
		if err != nil {
			return err
		}
		for _, user := range all {
			if user.ID <= afterID {
				continue
			}
			if len(users) == adminUserPageSize {
				hasMore = true
				break
			}
			users = append(users, user)
		}
	}

	text := strings.Builder{}
	if len(users) > 0 {
		for _, user := range users {
			text.WriteString(localized(a.language, "UserLine", userLineMessageData{
				ID:     user.ID,
				Name:   user.DisplayName,
				Status: user.Status,
			}))
		}
	} else {
		text.WriteString(localized(a.language, "UsersEmpty"))
	}
	if hasMore {
		status := "all"
		if filter.Status != nil {
			status = string(*filter.Status)
		}
		text.WriteString(localized(a.language, "UsersNext", struct {
			Status  string
			AfterID domain.UserID
		}{Status: status, AfterID: users[len(users)-1].ID}))
	}

	_, err := a.client.SendText(ctx, message.ChatID, text.String())
	return err
}

func (a *Admin) UserCard(ctx context.Context, message IncomingMessage, userID domain.UserID) error {
	if !a.authorized(message) {
		return a.reject(ctx, message.ChatID)
	}

	user, err := a.accounts.UserByID(ctx, userID)
	if err != nil {
		return err
	}

	text := localized(a.language, "UserCard", userCardMessageData{
		ID:         user.ID,
		Name:       user.DisplayName,
		Status:     user.Status,
		Fee:        formatAmountMinor(user.MonthlyFeeMinor),
		Currency:   user.Currency,
		NextCharge: user.NextChargeOn,
	})

	_, err = a.client.SendText(ctx, message.ChatID, text)
	return err
}

func (a *Admin) CreateUser(ctx context.Context, message IncomingMessage, params account.CreateUserParams) (domain.User, error) {
	if !a.authorized(message) {
		return domain.User{}, ErrAdminOnly
	}

	params.AdminTelegramID = message.UserID
	return a.accounts.CreateUser(ctx, params)
}

func (a *Admin) BeginPayment(ctx context.Context, message IncomingMessage, draft PaymentDraft) error {
	if !a.authorized(message) {
		return a.reject(ctx, message.ChatID)
	}

	user, err := a.accounts.UserByID(ctx, draft.UserID)
	if err != nil {
		return err
	}
	if draft.OccurredAt.IsZero() {
		draft.OccurredAt = a.now().UTC().Truncate(time.Second)
	}
	draft, err = a.wizard.BeginPayment(ctx, message.UserID, draft)
	if err != nil {
		return err
	}
	note := "-"
	if draft.Note != nil && strings.TrimSpace(*draft.Note) != "" {
		note = strings.TrimSpace(*draft.Note)
	}

	text := localized(a.language, "PaymentConfirm", paymentConfirmMessageData{
		UserID:         draft.UserID,
		UserName:       user.DisplayName,
		Amount:         formatAmountMinor(draft.AmountMinor),
		Currency:       user.Currency,
		OccurredAt:     draft.OccurredAt.UTC().Format("2006-01-02 15:04 UTC"),
		Note:           note,
		ConfirmCommand: "/admin confirm",
		CancelCommand:  "/admin cancel",
	})

	_, err = a.client.SendText(ctx, message.ChatID, text)
	return err
}

func (a *Admin) ConfirmPayment(ctx context.Context, message IncomingMessage) error {
	if !a.authorized(message) {
		return a.reject(ctx, message.ChatID)
	}

	if err := a.wizard.ConfirmPayment(ctx, message.UserID, a.accounts); err != nil {
		return err
	}
	_, err := a.client.SendText(ctx, message.ChatID, localized(a.language, "PaymentRecorded"))
	return err
}

func (a *Admin) CancelWizard(ctx context.Context, message IncomingMessage) error {
	if !a.authorized(message) {
		return a.reject(ctx, message.ChatID)
	}

	if err := a.wizard.Cancel(ctx, message.UserID); err != nil {
		return err
	}

	_, err := a.client.SendText(ctx, message.ChatID, localized(a.language, "WizardCancelled"))
	return err
}

func (a *Admin) Pause(ctx context.Context, message IncomingMessage, userID domain.UserID) error {
	if !a.authorized(message) {
		return a.reject(ctx, message.ChatID)
	}

	if _, err := a.accounts.Pause(ctx, account.AdminUserParams{AdminTelegramID: message.UserID, UserID: userID}); err != nil {
		return err
	}
	return a.completed(ctx, message.ChatID)
}

func (a *Admin) Disable(ctx context.Context, message IncomingMessage, userID domain.UserID) error {
	if !a.authorized(message) {
		return a.reject(ctx, message.ChatID)
	}

	if _, err := a.accounts.Disable(ctx, account.AdminUserParams{AdminTelegramID: message.UserID, UserID: userID}); err != nil {
		return err
	}
	return a.completed(ctx, message.ChatID)
}

func (a *Admin) Resume(ctx context.Context, message IncomingMessage, params account.ResumeParams) error {
	if !a.authorized(message) {
		return a.reject(ctx, message.ChatID)
	}

	params.AdminTelegramID = message.UserID
	if _, err := a.accounts.Resume(ctx, params); err != nil {
		return err
	}
	return a.completed(ctx, message.ChatID)
}

func (a *Admin) ChangeFee(ctx context.Context, message IncomingMessage, params account.ChangeMonthlyFeeParams) error {
	if !a.authorized(message) {
		return a.reject(ctx, message.ChatID)
	}

	params.AdminTelegramID = message.UserID
	if _, err := a.accounts.ChangeMonthlyFee(ctx, params); err != nil {
		return err
	}
	return a.completed(ctx, message.ChatID)
}

func (a *Admin) AddOpeningBalance(ctx context.Context, message IncomingMessage, params account.AddOpeningBalanceParams) error {
	if !a.authorized(message) {
		return a.reject(ctx, message.ChatID)
	}

	params.AdminTelegramID = message.UserID

	if _, err := a.accounts.AddOpeningBalance(ctx, params); err != nil {
		return err
	}
	return a.completed(ctx, message.ChatID)
}

func (a *Admin) AddAdjustment(ctx context.Context, message IncomingMessage, params account.AddAdjustmentParams) error {
	if !a.authorized(message) {
		return a.reject(ctx, message.ChatID)
	}

	params.AdminTelegramID = message.UserID

	if _, err := a.accounts.AddAdjustment(ctx, params); err != nil {
		return err
	}
	return a.completed(ctx, message.ChatID)
}

func (a *Admin) Reverse(ctx context.Context, message IncomingMessage, params account.ReverseLedgerEntryParams) error {
	if !a.authorized(message) {
		return a.reject(ctx, message.ChatID)
	}

	params.AdminTelegramID = message.UserID

	if _, err := a.accounts.ReverseLedgerEntry(ctx, params); err != nil {
		return err
	}
	return a.completed(ctx, message.ChatID)
}

func (a *Admin) completed(ctx context.Context, chatID int64) error {
	_, err := a.client.SendText(ctx, chatID, localized(a.language, "AdminActionCompleted"))
	return err
}

func paymentDraftStore(accounts AdminAccountService) PaymentDraftStore {
	store, _ := accounts.(PaymentDraftStore)
	return store
}

// HandleAdminCommand is the production command boundary for admin scenarios.
// Supported forms: /admin; /admin users [status]; /admin invite <user-id>;
// /admin payment <user-id> <amount> [note]; /admin confirm; /admin cancel.
func (b *Bot) HandleAdminCommand(ctx context.Context, message IncomingMessage) error {
	if b.admin == nil {
		return b.send(ctx, message.ChatID, localized(b.language, "AdminUnconfigured"))
	}

	if !b.admin.authorized(message) {
		return b.admin.reject(ctx, message.ChatID)
	}

	parts := strings.Fields(message.Text)

	var err error
	if len(parts) <= 1 {
		err = b.admin.Dashboard(ctx, message)
	} else {
		switch parts[1] {
		case "users":
			err = b.handleUsersCommand(ctx, message, parts)
		case "invite":
			err = b.handleInviteCommand(ctx, message, parts)
		case "user":
			err = b.handleUserCommand(ctx, message, parts)
		case "create":
			err = b.handleCreateCommand(ctx, message, parts)
		case "pause", "disable":
			err = b.handlePauseDisableCommand(ctx, message, parts)
		case "resume":
			err = b.handleResumeCommand(ctx, message, parts)
		case "fee":
			err = b.handleFeeCommand(ctx, message, parts)
		case "opening", "adjustment":
			err = b.handleOpeningAdjustmentCommand(ctx, message, parts)
		case "reverse":
			err = b.handleReverseCommand(ctx, message, parts)
		case "payment":
			err = b.handlePaymentCommand(ctx, message, parts)
		case "confirm":
			err = b.admin.ConfirmPayment(ctx, message)
		case "cancel":
			err = b.admin.CancelWizard(ctx, message)
		case "remind":
			err = b.handleRemindCommand(ctx, message, parts)
		case "reconcile":
			err = b.handleReconcileCommand(ctx, message, parts)
		default:
			return b.send(ctx, message.ChatID, localized(b.language, "AdminHelp"))
		}
	}
	if key, known := adminErrorLocalizationKey(err); known {
		return b.send(ctx, message.ChatID, localized(b.language, key))
	}
	return err
}

func adminErrorLocalizationKey(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	switch {
	case errors.Is(err, ErrWizardNotFound):
		return "AdminWizardMissing", true
	case errors.Is(err, account.ErrNotFound), errors.Is(err, account.ErrLedgerEntryNotFound), errors.Is(err, account.ErrInviteNotFound):
		return "AdminUserNotFound", true
	case errors.Is(err, account.ErrInvalidUserStatusTransition), errors.Is(err, account.ErrResumeDateRequired):
		return "AdminInvalidState", true
	case errors.Is(err, account.ErrLedgerEntryAlreadyReversed), errors.Is(err, account.ErrReversalUserMismatch), errors.Is(err, account.ErrCannotReverseReversal):
		return "AdminLedgerConflict", true
	case errors.Is(err, account.ErrInviteUserLinked), errors.Is(err, account.ErrTelegramUserIDTaken), errors.Is(err, account.ErrTelegramChatIDTaken):
		return "AdminInviteLinked", true
	case errors.Is(err, account.ErrInvalidDisplayName),
		errors.Is(err, account.ErrInvalidMonthlyFee),
		errors.Is(err, account.ErrUnsupportedCurrency),
		errors.Is(err, account.ErrInvalidAnchorDay),
		errors.Is(err, account.ErrInvalidNextChargeOn),
		errors.Is(err, account.ErrInvalidUserStatus),
		errors.Is(err, account.ErrInvalidInviteToken),
		errors.Is(err, account.ErrInvalidLedgerAmount),
		errors.Is(err, account.ErrInvalidPaymentAmount),
		errors.Is(err, account.ErrInvalidAdminTelegramID),
		errors.Is(err, account.ErrAdjustmentNoteRequired),
		errors.Is(err, account.ErrReversalNoteRequired),
		errors.Is(err, domain.ErrAmountOverflow),
		errors.Is(err, domain.ErrInvalidDate):
		return "AdminInvalidValue", true
	default:
		return "", false
	}
}

func (b *Bot) handleUsersCommand(ctx context.Context, message IncomingMessage, parts []string) error {
	var filter account.UserFilter
	var afterID domain.UserID

	if len(parts) >= 3 && parts[2] != "all" {
		status := domain.UserStatus(parts[2])
		if !status.IsValid() {
			return b.send(ctx, message.ChatID, localized(b.language, "StatusInvalid"))
		}

		filter.Status = &status
	}
	if len(parts) >= 4 {
		parsed, err := strconv.ParseInt(parts[3], 10, 64)
		if err != nil || parsed <= 0 {
			return b.send(ctx, message.ChatID, localized(b.language, "UserIDInvalid"))
		}
		afterID = domain.UserID(parsed)
	}
	if len(parts) > 4 {
		return b.send(ctx, message.ChatID, localized(b.language, "AdminHelp"))
	}

	return b.admin.Users(ctx, message, filter, afterID)
}

func (b *Bot) handleInviteCommand(ctx context.Context, message IncomingMessage, parts []string) error {
	userID, err := adminUserID(b.language, parts, 2)
	if err != nil {
		return b.send(ctx, message.ChatID, err.Error())
	}

	return b.admin.CreateInvite(ctx, message, userID)
}

func (b *Bot) handleUserCommand(ctx context.Context, message IncomingMessage, parts []string) error {
	userID, err := adminUserID(b.language, parts, 2)
	if err != nil {
		return b.send(ctx, message.ChatID, err.Error())
	}

	return b.admin.UserCard(ctx, message, userID)
}

func (b *Bot) handleCreateCommand(ctx context.Context, message IncomingMessage, parts []string) error {
	if len(parts) != 6 {
		return b.send(ctx, message.ChatID, localized(b.language, "CreateUsage"))
	}

	fee, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return b.send(ctx, message.ChatID, localized(b.language, "FeeInvalid"))
	}

	anchor, err := strconv.Atoi(parts[4])
	if err != nil {
		return b.send(ctx, message.ChatID, localized(b.language, "AnchorInvalid"))
	}

	next, err := domain.ParseDate(parts[5])
	if err != nil {
		return b.send(ctx, message.ChatID, localized(b.language, "NextChargeInvalid"))
	}

	user, err := b.admin.CreateUser(ctx, message, account.CreateUserParams{
		DisplayName:      parts[2],
		MonthlyFeeMinor:  domain.AmountMinor(fee),
		Currency:         "RUB",
		BillingAnchorDay: anchor,
		NextChargeOn:     next,
	})
	if err != nil {
		return err
	}
	return b.send(ctx, message.ChatID, localized(b.language, "AdminUserCreated", struct {
		ID   domain.UserID
		Name string
	}{ID: user.ID, Name: user.DisplayName}))
}

func (b *Bot) handlePauseDisableCommand(ctx context.Context, message IncomingMessage, parts []string) error {
	userID, err := adminUserID(b.language, parts, 2)
	if err != nil {
		return b.send(ctx, message.ChatID, err.Error())
	}

	if parts[1] == "pause" {
		return b.admin.Pause(ctx, message, userID)
	}

	return b.admin.Disable(ctx, message, userID)
}

func (b *Bot) handleResumeCommand(ctx context.Context, message IncomingMessage, parts []string) error {
	userID, err := adminUserID(b.language, parts, 2)
	if err != nil {
		return b.send(ctx, message.ChatID, err.Error())
	}

	if len(parts) != 4 {
		return b.send(ctx, message.ChatID, localized(b.language, "ResumeUsage"))
	}

	next, err := domain.ParseDate(parts[3])
	if err != nil {
		return b.send(ctx, message.ChatID, localized(b.language, "NextChargeInvalid"))
	}

	return b.admin.Resume(ctx, message, account.ResumeParams{
		UserID:       userID,
		NextChargeOn: &next,
	})
}

func (b *Bot) handleFeeCommand(ctx context.Context, message IncomingMessage, parts []string) error {
	userID, err := adminUserID(b.language, parts, 2)
	if err != nil {
		return b.send(ctx, message.ChatID, err.Error())
	}

	if len(parts) != 4 {
		return b.send(ctx, message.ChatID, localized(b.language, "FeeUsage"))
	}

	fee, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return b.send(ctx, message.ChatID, localized(b.language, "AmountInvalid"))
	}

	return b.admin.ChangeFee(ctx, message, account.ChangeMonthlyFeeParams{
		UserID:          userID,
		MonthlyFeeMinor: domain.AmountMinor(fee),
	})
}

func (b *Bot) handleOpeningAdjustmentCommand(ctx context.Context, message IncomingMessage, parts []string) error {
	userID, err := adminUserID(b.language, parts, 2)
	if err != nil {
		return b.send(ctx, message.ChatID, err.Error())
	}

	if len(parts) < 4 {
		return b.send(ctx, message.ChatID, localized(b.language, "AmountRequired"))
	}

	amount, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return b.send(ctx, message.ChatID, localized(b.language, "AmountInvalid"))
	}

	note := strings.TrimSpace(strings.Join(parts[4:], " "))
	if parts[1] == "opening" {
		var p *string
		if note != "" {
			p = &note
		}

		return b.admin.AddOpeningBalance(ctx, message, account.AddOpeningBalanceParams{
			UserID:      userID,
			AmountMinor: domain.AmountMinor(amount),
			Note:        p,
		})
	}

	return b.admin.AddAdjustment(ctx, message, account.AddAdjustmentParams{
		UserID:      userID,
		AmountMinor: domain.AmountMinor(amount),
		Note:        note,
	})
}

func (b *Bot) handleReverseCommand(ctx context.Context, message IncomingMessage, parts []string) error {
	userID, err := adminUserID(b.language, parts, 2)
	if err != nil {
		return b.send(ctx, message.ChatID, err.Error())
	}

	if len(parts) < 5 {
		return b.send(ctx, message.ChatID, localized(b.language, "ReverseUsage"))
	}

	entryID, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return b.send(ctx, message.ChatID, localized(b.language, "EntryIDInvalid"))
	}

	return b.admin.Reverse(ctx, message, account.ReverseLedgerEntryParams{
		UserID:  userID,
		EntryID: entryID,
		Note:    strings.Join(parts[4:], " "),
	})
}

func (b *Bot) handlePaymentCommand(ctx context.Context, message IncomingMessage, parts []string) error {
	if len(parts) < 4 {
		return b.send(ctx, message.ChatID, localized(b.language, "PaymentUsage"))
	}

	userID, err := adminUserID(b.language, parts, 2)
	if err != nil {
		return b.send(ctx, message.ChatID, err.Error())
	}

	amount, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || amount <= 0 {
		return b.send(ctx, message.ChatID, localized(b.language, "AmountInvalid"))
	}

	var note *string
	if value := strings.TrimSpace(strings.Join(parts[4:], " ")); value != "" {
		note = &value
	}

	return b.admin.BeginPayment(ctx, message, PaymentDraft{
		UserID:      userID,
		AmountMinor: domain.AmountMinor(amount),
		Note:        note,
	})
}

func (b *Bot) handleRemindCommand(ctx context.Context, message IncomingMessage, parts []string) error {
	userID, err := adminUserID(b.language, parts, 2)
	if err != nil {
		return b.send(ctx, message.ChatID, err.Error())
	}

	text := strings.TrimSpace(strings.Join(parts[3:], " "))
	if text == "" {
		return b.send(ctx, message.ChatID, localized(b.language, "RemindUsage"))
	}

	return b.admin.RemindNow(ctx, message, userID, text)
}

func (b *Bot) handleReconcileCommand(ctx context.Context, message IncomingMessage, parts []string) error {
	if len(parts) < 5 || len(parts) > 6 {
		return b.send(ctx, message.ChatID, localized(b.language, "ReconcileUsage"))
	}
	userID, err := adminUserID(b.language, parts, 2)
	if err != nil {
		return b.send(ctx, message.ChatID, err.Error())
	}
	billingDate, err := domain.ParseDate(parts[3])
	if err != nil {
		return b.send(ctx, message.ChatID, localized(b.language, "NextChargeInvalid"))
	}
	reminderType := domain.ReminderType(parts[4])
	if !reminderType.IsValid() {
		return b.send(ctx, message.ChatID, localized(b.language, "ReminderTypeInvalid"))
	}
	deliveryKey := ""
	if len(parts) == 6 {
		deliveryKey = parts[5]
	}
	if reminderType == domain.ReminderTypeManual && deliveryKey == "" {
		return b.send(ctx, message.ChatID, localized(b.language, "ReconcileUsage"))
	}
	return b.admin.ReconcileReminder(ctx, message, userID, billingDate, reminderType, deliveryKey)
}

func adminUserID(language localization.Language, parts []string, index int) (domain.UserID, error) {
	if len(parts) <= index {
		return 0, errors.New(localized(language, "UserIDMissing"))
	}

	value, err := strconv.ParseInt(parts[index], 10, 64)
	if err != nil || value <= 0 {
		return 0, errors.New(localized(language, "UserIDInvalid"))
	}

	return domain.UserID(value), nil
}
