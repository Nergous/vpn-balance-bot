package telegram

import (
	"fmt"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

const telegramTextLimit = 4096

func formatAmountMinor(amount domain.AmountMinor) string {
	value := amount.Int64()
	sign := ""
	var magnitude uint64
	if value < 0 {
		sign = "-"
		magnitude = uint64(-(value + 1)) + 1
	} else {
		magnitude = uint64(value)
	}
	return fmt.Sprintf("%s%d.%02d", sign, magnitude/100, magnitude%100)
}

func formatSignedAmountMinor(amount domain.AmountMinor) string {
	formatted := formatAmountMinor(amount)
	if amount > 0 {
		return "+" + formatted
	}
	return formatted
}

func splitTelegramText(text string) []string {
	runes := []rune(text)
	if len(runes) <= telegramTextLimit {
		return []string{text}
	}

	parts := make([]string, 0, len(runes)/telegramTextLimit+1)
	for len(runes) > telegramTextLimit {
		splitAt := telegramTextLimit
		for index := telegramTextLimit - 1; index > 0; index-- {
			if runes[index] == '\n' {
				splitAt = index + 1
				break
			}
		}
		parts = append(parts, string(runes[:splitAt]))
		runes = runes[splitAt:]
	}
	if len(runes) > 0 {
		parts = append(parts, string(runes))
	}
	return parts
}
