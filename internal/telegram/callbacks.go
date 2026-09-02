package telegram

import "context"

// HandleCallback dispatches a supported customer callback without trusting external IDs.
func (b *Bot) HandleCallback(ctx context.Context, callback IncomingCallback) error {
	if !callback.isPrivate() {
		return ErrPrivateChatRequired
	}

	switch callback.Data {
	case "user_status":
		return b.HandleStatus(ctx, callback.message())
	case "user_history":
		return b.HandleHistory(ctx, callback.message())
	case "user_help":
		return b.HandleHelp(ctx, callback.message())
	default:
		return b.send(ctx, callback.ChatID, localized(b.language, "CallbackUnknown"))
	}
}

func (c IncomingCallback) message() IncomingMessage {
	return IncomingMessage{
		UpdateID:  c.UpdateID,
		MessageID: c.MessageID,
		ChatID:    c.ChatID,
		UserID:    c.UserID,
		ChatType:  c.ChatType,
	}
}
