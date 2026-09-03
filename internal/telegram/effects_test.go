package telegram

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
)

func TestStateChangingUpdateSendsOnlyAfterTransactionCommit(t *testing.T) {
	processor := &recordingUpdateProcessor{fakeAdmin: &fakeAdmin{}, process: true}
	client := &transactionAwareClient{processor: processor}
	bot := NewWithClient(client, processor, LanguageEnglish)

	update := messageUpdate(42, 1, "/start invite-token")
	if err := bot.processUpdate(context.Background(), update, nil); err != nil {
		t.Fatal(err)
	}
	if processor.calls != 1 || client.sendCalls != 1 {
		t.Fatalf("processor calls=%d send calls=%d", processor.calls, client.sendCalls)
	}
	if client.calledInsideTransaction {
		t.Fatal("Telegram send occurred before update transaction committed")
	}
}

func TestDuplicateStateChangingUpdateDoesNotRepeatPostCommitEffect(t *testing.T) {
	processor := &recordingUpdateProcessor{fakeAdmin: &fakeAdmin{}, process: false}
	client := &transactionAwareClient{processor: processor}
	bot := NewWithClient(client, processor, LanguageEnglish)

	if err := bot.processUpdate(context.Background(), messageUpdate(42, 1, "/start invite-token"), nil); err != nil {
		t.Fatal(err)
	}
	if processor.calls != 1 || client.sendCalls != 0 {
		t.Fatalf("processor calls=%d send calls=%d", processor.calls, client.sendCalls)
	}
}

func TestPostCommitResponseFailureDoesNotRetryCommittedUpdate(t *testing.T) {
	processor := &recordingUpdateProcessor{fakeAdmin: &fakeAdmin{}, process: true}
	client := &transactionAwareClient{processor: processor, err: errors.New("send failed")}
	bot := NewWithClient(client, processor, LanguageEnglish)

	if err := bot.processUpdate(context.Background(), messageUpdate(42, 1, "/start invite-token"), nil); err != nil {
		t.Fatalf("processUpdate() error = %v", err)
	}
	if processor.calls != 1 || client.sendCalls != 1 {
		t.Fatalf("processor calls=%d send calls=%d", processor.calls, client.sendCalls)
	}
}

func TestManualReminderUsesDeliveryIdempotencyAndIgnoresAcknowledgementFailure(t *testing.T) {
	processor := &recordingUpdateProcessor{fakeAdmin: &fakeAdmin{}, process: true}
	client := &transactionAwareClient{processor: processor, err: errors.New("admin acknowledgement failed")}
	reminders := &fakeAdminReminders{}
	bot := NewWithClient(client, processor, LanguageEnglish)
	if err := bot.EnableAdmin(1, reminders); err != nil {
		t.Fatal(err)
	}

	if err := bot.processUpdate(context.Background(), messageUpdate(43, 1, "/admin remind 7 top up"), nil); err != nil {
		t.Fatalf("processUpdate() error = %v", err)
	}
	if processor.calls != 0 {
		t.Fatalf("manual reminder used general update transaction %d times", processor.calls)
	}
	if reminders.calls != 1 || client.sendCalls != 1 {
		t.Fatalf("reminder calls=%d acknowledgement calls=%d", reminders.calls, client.sendCalls)
	}
}

func TestReminderSendRejectsTransactionalContext(t *testing.T) {
	bot := NewWithClient(&transactionAwareClient{}, &fakeAdmin{}, LanguageEnglish)
	ctx, _ := withPostCommitQueue(context.Background())
	if _, err := bot.SendReminder(ctx, 1, "message"); !errors.Is(err, ErrTransactionalSend) {
		t.Fatalf("SendReminder() error = %v", err)
	}
}

func TestNewRejectsHTTPTimeoutThatDisablesLongPolling(t *testing.T) {
	if _, err := New("token", &fakeAdmin{}, LanguageEnglish, 1500*time.Millisecond, nil); !errors.Is(err, ErrInvalidHTTPTimeout) {
		t.Fatalf("New() error = %v", err)
	}
}

type recordingUpdateProcessor struct {
	*fakeAdmin
	process       bool
	inTransaction bool
	calls         int
}

func (p *recordingUpdateProcessor) ProcessTelegramUpdate(ctx context.Context, _ int64, handler func(context.Context) error) (bool, error) {
	p.calls++
	if !p.process {
		return false, nil
	}
	p.inTransaction = true
	err := handler(ctx)
	p.inTransaction = false
	return err == nil, err
}

type transactionAwareClient struct {
	processor               *recordingUpdateProcessor
	sendCalls               int
	calledInsideTransaction bool
	err                     error
}

func (c *transactionAwareClient) SendText(context.Context, int64, string) (int, error) {
	c.recordCall()
	return c.sendCalls, c.err
}

func (c *transactionAwareClient) SendInviteToken(context.Context, int64, string, string, string) (int, error) {
	c.recordCall()
	return c.sendCalls, c.err
}

func (c *transactionAwareClient) recordCall() {
	c.sendCalls++
	if c.processor != nil && c.processor.inTransaction {
		c.calledInsideTransaction = true
	}
}

func messageUpdate(updateID, userID int64, text string) *models.Update {
	return &models.Update{
		ID: updateID,
		Message: &models.Message{
			ID:   1,
			Chat: models.Chat{ID: userID, Type: models.ChatTypePrivate},
			From: &models.User{ID: userID},
			Text: text,
		},
	}
}
