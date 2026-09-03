package telegram

import (
	"context"
	"errors"
)

type postCommitQueueKey struct{}

type postCommitQueue struct {
	effects []func(context.Context) error
}

func withPostCommitQueue(ctx context.Context) (context.Context, *postCommitQueue) {
	queue := &postCommitQueue{}
	return context.WithValue(ctx, postCommitQueueKey{}, queue), queue
}

func postCommitQueueFromContext(ctx context.Context) *postCommitQueue {
	if ctx == nil {
		return nil
	}
	queue, _ := ctx.Value(postCommitQueueKey{}).(*postCommitQueue)
	return queue
}

func (q *postCommitQueue) flush(ctx context.Context) error {
	var errs []error
	for _, effect := range q.effects {
		if err := effect(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type postCommitClient struct {
	base Client
}

func deferClient(client Client) Client {
	if client == nil {
		return nil
	}
	if _, ok := client.(*postCommitClient); ok {
		return client
	}
	return &postCommitClient{base: client}
}

func (c *postCommitClient) SendText(ctx context.Context, chatID int64, message string) (int, error) {
	if queue := postCommitQueueFromContext(ctx); queue != nil {
		queue.effects = append(queue.effects, func(effectCtx context.Context) error {
			_, err := c.base.SendText(effectCtx, chatID, message)
			return err
		})
		return 0, nil
	}
	return c.base.SendText(ctx, chatID, message)
}

func (c *postCommitClient) SendInviteToken(ctx context.Context, chatID int64, title, copyLabel, token string) (int, error) {
	if queue := postCommitQueueFromContext(ctx); queue != nil {
		queue.effects = append(queue.effects, func(effectCtx context.Context) error {
			_, err := c.base.SendInviteToken(effectCtx, chatID, title, copyLabel, token)
			return err
		})
		return 0, nil
	}
	return c.base.SendInviteToken(ctx, chatID, title, copyLabel, token)
}
