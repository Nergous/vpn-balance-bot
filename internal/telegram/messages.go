package telegram

import "github.com/Nergous/vpn-balance-bot/internal/localization"

const (
	// LanguageRussian selects Russian message text.
	LanguageRussian = localization.Russian
	// LanguageEnglish selects English message text.
	LanguageEnglish = localization.English
)

// localized reads a stable message ID from embedded JSON catalogs.
func localized(language localization.Language, id string, data ...any) string {
	var templateData any
	if len(data) > 0 {
		templateData = data[0]
	}
	return localization.MustNew(language).Text(id, templateData)
}
