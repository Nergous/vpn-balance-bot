package account

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

func TestNewRejectsNilStorage(t *testing.T) {
	service, err := New(nil, time.Hour)
	if !errors.Is(err, ErrNilStorage) {
		t.Fatalf("New(nil) error = %v, want %v", err, ErrNilStorage)
	}
	if service != nil {
		t.Fatalf("New(nil) service = %#v, want nil", service)
	}
}

func TestCreateUserValidatesParams(t *testing.T) {
	nextChargeOn := testDate(t, 2026, time.September, 1)
	tests := []struct {
		name   string
		params CreateUserParams
		want   error
	}{
		{name: "blank display name", params: CreateUserParams{MonthlyFeeMinor: 1, Currency: "RUB", BillingAnchorDay: 1, NextChargeOn: nextChargeOn}, want: ErrInvalidDisplayName},
		{name: "zero fee", params: CreateUserParams{DisplayName: "Alice", Currency: "RUB", BillingAnchorDay: 1, NextChargeOn: nextChargeOn}, want: ErrInvalidMonthlyFee},
		{name: "unsupported currency", params: CreateUserParams{DisplayName: "Alice", MonthlyFeeMinor: 1, Currency: "USD", BillingAnchorDay: 1, NextChargeOn: nextChargeOn}, want: ErrUnsupportedCurrency},
		{name: "invalid anchor", params: CreateUserParams{DisplayName: "Alice", MonthlyFeeMinor: 1, Currency: "RUB", BillingAnchorDay: 32, NextChargeOn: nextChargeOn}, want: ErrInvalidAnchorDay},
		{name: "invalid date", params: CreateUserParams{DisplayName: "Alice", MonthlyFeeMinor: 1, Currency: "RUB", BillingAnchorDay: 1, NextChargeOn: domain.Date{Year: 2026, Month: time.February, Day: 30}}, want: ErrInvalidNextChargeOn},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			storage := &fakeStorage{}
			service := newService(storage, time.Hour, func() time.Time { return time.Time{} })
			_, err := service.CreateUser(context.Background(), test.params)
			if !errors.Is(err, test.want) {
				t.Fatalf("CreateUser() error = %v, want %v", err, test.want)
			}
			if storage.createUserCalled {
				t.Fatal("CreateUser() called storage after validation failure")
			}
		})
	}
}

func TestCreateUserBuildsActiveProfile(t *testing.T) {
	now := time.Date(2026, time.August, 28, 10, 0, 0, 123000000, time.FixedZone("test", 3*60*60))
	telegramUserID := int64(101)
	telegramChatID := int64(202)
	username := "alice"
	nextChargeOn := testDate(t, 2026, time.September, 17)

	storage := &fakeStorage{
		createUser: func(_ context.Context, user domain.User) (domain.User, error) {
			user.ID = 7
			return user, nil
		},
	}
	service := newService(storage, time.Hour, func() time.Time { return now })

	created, err := service.CreateUser(context.Background(), CreateUserParams{
		TelegramUserID:   &telegramUserID,
		TelegramChatID:   &telegramChatID,
		Username:         &username,
		DisplayName:      "  Alice  ",
		MonthlyFeeMinor:  150000,
		Currency:         "RUB",
		BillingAnchorDay: 17,
		NextChargeOn:     nextChargeOn,
	})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	want := domain.User{
		ID:               7,
		TelegramUserID:   &telegramUserID,
		TelegramChatID:   &telegramChatID,
		Username:         &username,
		DisplayName:      "Alice",
		MonthlyFeeMinor:  150000,
		Currency:         "RUB",
		BillingAnchorDay: 17,
		NextChargeOn:     nextChargeOn,
		Status:           domain.UserStatusActive,
		CreatedAt:        now.UTC().Truncate(time.Second),
		UpdatedAt:        now.UTC().Truncate(time.Second),
	}
	storageWant := want
	storageWant.ID = 0
	if !reflect.DeepEqual(storage.createdUser, storageWant) {
		t.Fatalf("storage user = %#v, want %#v", storage.createdUser, storageWant)
	}
	if !reflect.DeepEqual(created, want) {
		t.Fatalf("CreateUser() = %#v, want %#v", created, want)
	}
}

func TestListUsersRejectsInvalidStatus(t *testing.T) {
	storage := &fakeStorage{}
	service := newService(storage, time.Hour, time.Now)
	status := domain.UserStatus("unknown")

	_, err := service.ListUsers(context.Background(), UserFilter{Status: &status})
	if !errors.Is(err, ErrInvalidUserStatus) {
		t.Fatalf("ListUsers() error = %v, want %v", err, ErrInvalidUserStatus)
	}
	if storage.listUsersCalled {
		t.Fatal("ListUsers() called storage after validation failure")
	}
}

func TestBoundedAdminQueriesUseStorageCapabilities(t *testing.T) {
	status := domain.UserStatusActive
	storage := &fakeAdminQueryStorage{
		fakeStorage: &fakeStorage{},
		listPage: func(_ context.Context, gotStatus *domain.UserStatus, afterID domain.UserID, limit int) ([]domain.User, bool, error) {
			if gotStatus == nil || *gotStatus != status || afterID != 7 || limit != 50 {
				t.Fatalf("ListUsersPage args = %#v, %d, %d", gotStatus, afterID, limit)
			}
			return []domain.User{{ID: 8, Status: status}}, true, nil
		},
		statusCounts: func(context.Context) (UserStatusCounts, error) {
			return UserStatusCounts{Total: 4, Active: 2, Paused: 1, Disabled: 1}, nil
		},
	}
	service := newService(storage, time.Hour, time.Now)

	users, hasMore, err := service.ListUsersPage(context.Background(), UserFilter{Status: &status}, 7, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].ID != 8 || !hasMore || storage.pageCalls != 1 {
		t.Fatalf("users=%#v hasMore=%v calls=%d", users, hasMore, storage.pageCalls)
	}
	counts, err := service.UserStatusCounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if counts != (UserStatusCounts{Total: 4, Active: 2, Paused: 1, Disabled: 1}) || storage.countCalls != 1 {
		t.Fatalf("counts=%#v calls=%d", counts, storage.countCalls)
	}
}

func TestListUsersPageRejectsInvalidLimit(t *testing.T) {
	service := newService(&fakeStorage{}, time.Hour, time.Now)
	_, _, err := service.ListUsersPage(context.Background(), UserFilter{}, 0, 0)
	if !errors.Is(err, ErrInvalidUserPageLimit) {
		t.Fatalf("ListUsersPage() error = %v, want %v", err, ErrInvalidUserPageLimit)
	}
}

func TestServiceForwardsReadAndProfileCommands(t *testing.T) {
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	nextChargeOn := testDate(t, 2026, time.September, 17)
	storedUser := domain.User{ID: 1, Status: domain.UserStatusActive}

	storage := &fakeStorage{
		userByID: func(_ context.Context, userID domain.UserID) (domain.User, error) {
			if userID != 1 {
				t.Fatalf("UserByID userID = %d", userID)
			}
			return storedUser, nil
		},
		userByTelegramID: func(_ context.Context, telegramID int64) (domain.User, error) {
			if telegramID != 500 {
				t.Fatalf("UserByTelegramID ID = %d", telegramID)
			}
			return storedUser, nil
		},
		listUsers: func(_ context.Context, status *domain.UserStatus) ([]domain.User, error) {
			if status == nil || *status != domain.UserStatusActive {
				t.Fatalf("ListUsers status = %#v", status)
			}
			return []domain.User{storedUser}, nil
		},
		setMonthlyFee: func(_ context.Context, userID domain.UserID, fee domain.AmountMinor, updatedAt time.Time) (domain.User, error) {
			if userID != 1 || fee != 250000 || !updatedAt.Equal(now) {
				t.Fatalf("SetMonthlyFee args = %d, %d, %s", userID, fee, updatedAt)
			}
			return storedUser, nil
		},
		pauseUser: func(_ context.Context, userID domain.UserID, updatedAt time.Time) (domain.User, error) {
			if userID != 1 || !updatedAt.Equal(now) {
				t.Fatalf("PauseUser args = %d, %s", userID, updatedAt)
			}
			return storedUser, nil
		},
		resumeUser: func(_ context.Context, userID domain.UserID, date domain.Date, updatedAt time.Time) (domain.User, error) {
			if userID != 1 || date != nextChargeOn || !updatedAt.Equal(now) {
				t.Fatalf("ResumeUser args = %d, %s, %s", userID, date, updatedAt)
			}
			return storedUser, nil
		},
		disableUser: func(_ context.Context, userID domain.UserID, updatedAt time.Time) (domain.User, error) {
			if userID != 1 || !updatedAt.Equal(now) {
				t.Fatalf("DisableUser args = %d, %s", userID, updatedAt)
			}
			return storedUser, nil
		},
	}
	service := newService(storage, time.Hour, func() time.Time { return now })

	if _, err := service.UserByID(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := service.UserByTelegramID(context.Background(), 500); err != nil {
		t.Fatal(err)
	}
	status := domain.UserStatusActive
	if _, err := service.ListUsers(context.Background(), UserFilter{Status: &status}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ChangeMonthlyFee(context.Background(), ChangeMonthlyFeeParams{UserID: 1, MonthlyFeeMinor: 250000}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Pause(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resume(context.Background(), ResumeParams{UserID: 1, NextChargeOn: &nextChargeOn}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Disable(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
}

func TestServiceRejectsInvalidProfileCommands(t *testing.T) {
	service := newService(&fakeStorage{}, time.Hour, time.Now)

	_, err := service.ChangeMonthlyFee(context.Background(), ChangeMonthlyFeeParams{MonthlyFeeMinor: 0})
	if !errors.Is(err, ErrInvalidMonthlyFee) {
		t.Fatalf("ChangeMonthlyFee() error = %v, want %v", err, ErrInvalidMonthlyFee)
	}

	_, err = service.Resume(context.Background(), ResumeParams{UserID: 1})
	if !errors.Is(err, ErrResumeDateRequired) {
		t.Fatalf("Resume() error = %v, want %v", err, ErrResumeDateRequired)
	}

	invalidDate := domain.Date{Year: 2026, Month: time.February, Day: 30}
	_, err = service.Resume(context.Background(), ResumeParams{UserID: 1, NextChargeOn: &invalidDate})
	if !errors.Is(err, ErrInvalidNextChargeOn) {
		t.Fatalf("Resume() error = %v, want %v", err, ErrInvalidNextChargeOn)
	}
}

func TestServicePropagatesStorageErrors(t *testing.T) {
	storage := &fakeStorage{
		userByID: func(context.Context, domain.UserID) (domain.User, error) {
			return domain.User{}, ErrNotFound
		},
	}
	service := newService(storage, time.Hour, time.Now)

	_, err := service.UserByID(context.Background(), 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("UserByID() error = %v, want %v", err, ErrNotFound)
	}
}

type fakeStorage struct {
	createUserCalled      bool
	createdUser           domain.User
	createUser            func(context.Context, domain.User) (domain.User, error)
	userByID              func(context.Context, domain.UserID) (domain.User, error)
	listUsers             func(context.Context, *domain.UserStatus) ([]domain.User, error)
	userByTelegramID      func(context.Context, int64) (domain.User, error)
	setMonthlyFee         func(context.Context, domain.UserID, domain.AmountMinor, time.Time) (domain.User, error)
	pauseUser             func(context.Context, domain.UserID, time.Time) (domain.User, error)
	resumeUser            func(context.Context, domain.UserID, domain.Date, time.Time) (domain.User, error)
	disableUser           func(context.Context, domain.UserID, time.Time) (domain.User, error)
	createInviteToken     func(context.Context, CreateInviteTokenRecord) error
	consumeInviteToken    func(context.Context, ConsumeInviteTokenRecord) (domain.User, error)
	createLedgerEntry     func(context.Context, domain.LedgerEntry) (domain.LedgerEntry, error)
	balance               func(context.Context, domain.UserID) (domain.AmountMinor, error)
	lastLedgerEntries     func(context.Context, domain.UserID, int) ([]domain.LedgerEntry, error)
	lastUnreversedPayment func(context.Context, domain.UserID) (domain.LedgerEntry, bool, error)
	reverseLedgerEntry    func(context.Context, ReverseLedgerEntryRecord) (domain.LedgerEntry, error)
	listUsersCalled       bool
}

type fakeAdminQueryStorage struct {
	*fakeStorage
	listPage     func(context.Context, *domain.UserStatus, domain.UserID, int) ([]domain.User, bool, error)
	statusCounts func(context.Context) (UserStatusCounts, error)
	pageCalls    int
	countCalls   int
}

func (f *fakeAdminQueryStorage) ListUsersPage(ctx context.Context, status *domain.UserStatus, afterID domain.UserID, limit int) ([]domain.User, bool, error) {
	f.pageCalls++
	return f.listPage(ctx, status, afterID, limit)
}

func (f *fakeAdminQueryStorage) UserStatusCounts(ctx context.Context) (UserStatusCounts, error) {
	f.countCalls++
	return f.statusCounts(ctx)
}

func (f *fakeStorage) CreateUser(ctx context.Context, user domain.User) (domain.User, error) {
	f.createUserCalled = true
	f.createdUser = user
	if f.createUser == nil {
		return domain.User{}, errors.New("unexpected CreateUser call")
	}
	return f.createUser(ctx, user)
}

func (f *fakeStorage) UserByID(ctx context.Context, userID domain.UserID) (domain.User, error) {
	if f.userByID == nil {
		return domain.User{}, errors.New("unexpected UserByID call")
	}
	return f.userByID(ctx, userID)
}

func (f *fakeStorage) ListUsers(ctx context.Context, status *domain.UserStatus) ([]domain.User, error) {
	f.listUsersCalled = true
	if f.listUsers == nil {
		return nil, errors.New("unexpected ListUsers call")
	}
	return f.listUsers(ctx, status)
}

func (f *fakeStorage) UserByTelegramID(ctx context.Context, telegramID int64) (domain.User, error) {
	if f.userByTelegramID == nil {
		return domain.User{}, errors.New("unexpected UserByTelegramID call")
	}
	return f.userByTelegramID(ctx, telegramID)
}

func (f *fakeStorage) SetMonthlyFee(ctx context.Context, userID domain.UserID, fee domain.AmountMinor, updatedAt time.Time) (domain.User, error) {
	if f.setMonthlyFee == nil {
		return domain.User{}, errors.New("unexpected SetMonthlyFee call")
	}
	return f.setMonthlyFee(ctx, userID, fee, updatedAt)
}

func (f *fakeStorage) PauseUser(ctx context.Context, userID domain.UserID, updatedAt time.Time) (domain.User, error) {
	if f.pauseUser == nil {
		return domain.User{}, errors.New("unexpected PauseUser call")
	}
	return f.pauseUser(ctx, userID, updatedAt)
}

func (f *fakeStorage) ResumeUser(ctx context.Context, userID domain.UserID, nextChargeOn domain.Date, updatedAt time.Time) (domain.User, error) {
	if f.resumeUser == nil {
		return domain.User{}, errors.New("unexpected ResumeUser call")
	}
	return f.resumeUser(ctx, userID, nextChargeOn, updatedAt)
}

func (f *fakeStorage) DisableUser(ctx context.Context, userID domain.UserID, updatedAt time.Time) (domain.User, error) {
	if f.disableUser == nil {
		return domain.User{}, errors.New("unexpected DisableUser call")
	}
	return f.disableUser(ctx, userID, updatedAt)
}

func (f *fakeStorage) CreateInviteToken(ctx context.Context, record CreateInviteTokenRecord) error {
	if f.createInviteToken == nil {
		return errors.New("unexpected CreateInviteToken call")
	}
	return f.createInviteToken(ctx, record)
}

func (f *fakeStorage) ConsumeInviteToken(ctx context.Context, record ConsumeInviteTokenRecord) (domain.User, error) {
	if f.consumeInviteToken == nil {
		return domain.User{}, errors.New("unexpected ConsumeInviteToken call")
	}
	return f.consumeInviteToken(ctx, record)
}

func (f *fakeStorage) CreateLedgerEntry(ctx context.Context, entry domain.LedgerEntry) (domain.LedgerEntry, error) {
	if f.createLedgerEntry == nil {
		return domain.LedgerEntry{}, errors.New("unexpected CreateLedgerEntry call")
	}
	return f.createLedgerEntry(ctx, entry)
}

func (f *fakeStorage) Balance(ctx context.Context, userID domain.UserID) (domain.AmountMinor, error) {
	if f.balance == nil {
		return 0, errors.New("unexpected Balance call")
	}
	return f.balance(ctx, userID)
}

func (f *fakeStorage) LastLedgerEntries(ctx context.Context, userID domain.UserID, limit int) ([]domain.LedgerEntry, error) {
	if f.lastLedgerEntries == nil {
		return nil, errors.New("unexpected LastLedgerEntries call")
	}
	return f.lastLedgerEntries(ctx, userID, limit)
}

func (f *fakeStorage) LastUnreversedPayment(ctx context.Context, userID domain.UserID) (domain.LedgerEntry, bool, error) {
	if f.lastUnreversedPayment == nil {
		return domain.LedgerEntry{}, false, errors.New("unexpected LastUnreversedPayment call")
	}
	return f.lastUnreversedPayment(ctx, userID)
}

func (f *fakeStorage) ReverseLedgerEntry(ctx context.Context, record ReverseLedgerEntryRecord) (domain.LedgerEntry, error) {
	if f.reverseLedgerEntry == nil {
		return domain.LedgerEntry{}, errors.New("unexpected ReverseLedgerEntry call")
	}
	return f.reverseLedgerEntry(ctx, record)
}

func testDate(t *testing.T, year int, month time.Month, day int) domain.Date {
	t.Helper()
	date, err := domain.NewDate(year, month, day)
	if err != nil {
		t.Fatal(err)
	}
	return date
}
