package telegram

import (
	"math"
	"strings"
	"testing"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

func TestFormatAmountMinorUsesMajorCurrencyUnitsWithoutFloat(t *testing.T) {
	tests := map[domain.AmountMinor]string{
		0:                                 "0.00",
		1:                                 "0.01",
		25_000:                            "250.00",
		-25_000:                           "-250.00",
		domain.AmountMinor(math.MinInt64): "-92233720368547758.08",
	}
	for amount, want := range tests {
		if got := formatAmountMinor(amount); got != want {
			t.Errorf("formatAmountMinor(%d) = %q, want %q", amount, got, want)
		}
	}
}

func TestSplitTelegramTextPreservesContentAndLimit(t *testing.T) {
	text := strings.Repeat("я", telegramTextLimit-5) + "\n" + strings.Repeat("x", telegramTextLimit+20)
	parts := splitTelegramText(text)
	if len(parts) != 3 {
		t.Fatalf("parts = %d, want 3", len(parts))
	}
	if strings.Join(parts, "") != text {
		t.Fatal("split text did not preserve content")
	}
	for index, part := range parts {
		if length := len([]rune(part)); length > telegramTextLimit {
			t.Fatalf("part %d length = %d", index, length)
		}
	}
}
