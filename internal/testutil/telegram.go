package testutil

import (
	"context"
	"sync"
)

type TelegramSentMessage struct {
	ChatID int64
	Text   string
}
type FakeTelegramClient struct {
	mu   sync.Mutex
	Sent []TelegramSentMessage
	Err  error
}

func (c *FakeTelegramClient) SendText(_ context.Context, chatID int64, text string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Err != nil {
		return 0, c.Err
	}
	c.Sent = append(c.Sent, TelegramSentMessage{ChatID: chatID, Text: text})
	return len(c.Sent), nil
}
