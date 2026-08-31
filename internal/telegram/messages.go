package telegram

import "fmt"

const (
	LanguageRussian = "ru"
	LanguageEnglish = "en"
)

func normalizeLanguage(language string) string {
	if language == LanguageEnglish {
		return LanguageEnglish
	}
	return LanguageRussian
}

func localized(language, key string, args ...any) string {
	translations := map[string][2]string{
		"start_bound":        {"Профиль привязан. Используйте /status.", "Profile linked. Use /status."},
		"start_welcome":      {"VPN Balance Bot\n/status — мой статус\n/history — история\n/help — помощь", "VPN Balance Bot\n/status — my status\n/history — history\n/help — help"},
		"unlinked_status":    {"Профиль не привязан. Используйте invite link от администратора.", "Profile is not linked. Use an invite link from the administrator."},
		"unlinked_history":   {"Профиль не привязан.", "Profile is not linked."},
		"help":               {"Команды: /status, /history, /help", "Commands: /status, /history, /help"},
		"invite_expired":     {"Invite link истёк. Запросите новый у администратора.", "Invite link has expired. Request a new one from the administrator."},
		"invite_used":        {"Invite link уже использован.", "Invite link has already been used."},
		"invite_invalid":     {"Invite link недействителен.", "Invite link is invalid."},
		"invite_taken":       {"Этот Telegram account уже привязан к другому профилю.", "This Telegram account is already linked to another profile."},
		"invite_error":       {"Не удалось привязать профиль. Повторите позже.", "Could not link profile. Try again later."},
		"history_empty":      {"Операций пока нет.", "No transactions yet."},
		"callback_unknown":   {"Неизвестное действие.", "Unknown action."},
		"admin_denied":       {"Действие доступно только администратору.", "This action is available only to the administrator."},
		"admin_unconfigured": {"Админ-панель не настроена.", "Admin panel is not configured."},
		"users_empty":        {"Пользователей нет.", "No users."},
		"wizard_cancelled":   {"Операция отменена.", "Operation cancelled."},
		"reminder_before":    {"Списание за подписку будет через 3 дня.", "Your subscription charge is due in 3 days."},
		"reminder_charge":    {"Подписка списана. Пополните баланс.", "Your subscription charge was applied. Please top up your balance."},
		"reminder_overdue_3": {"Долг сохраняется уже 3 дня. Пополните баланс.", "Your balance has been overdue for 3 days. Please top up your balance."},
		"reminder_overdue_7": {"Долг сохраняется уже 7 дней. Пополните баланс.", "Your balance has been overdue for 7 days. Please top up your balance."},
		"reminder_default":   {"Пополните баланс.", "Please top up your balance."},
	}
	value, ok := translations[key]
	if !ok {
		return key
	}
	index := 0
	if normalizeLanguage(language) == LanguageEnglish {
		index = 1
	}
	return fmt.Sprintf(value[index], args...)
}

func localizedPair(language, russian, english string) string {
	if normalizeLanguage(language) == LanguageEnglish {
		return english
	}
	return russian
}
