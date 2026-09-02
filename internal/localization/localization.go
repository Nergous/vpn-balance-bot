package localization

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	textlanguage "golang.org/x/text/language"
)

// Language is a canonical BCP 47 language tag backed by an embedded catalog.
type Language string

const (
	Russian Language = "ru"
	English Language = "en"
	Default          = Russian
)

// ErrUnsupportedLanguage indicates that no embedded catalog matches a language.
var ErrUnsupportedLanguage = errors.New("unsupported language")

//go:embed locales/active.*.json
var files embed.FS

var bundle, supportedLanguages = loadBundle()

// Localizer formats messages from the embedded translation catalogs.
type Localizer struct {
	localizer *i18n.Localizer
}

// Parse validates and canonicalizes a language code against embedded catalogs.
func Parse(value string) (Language, error) {
	tag, err := textlanguage.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrUnsupportedLanguage, value)
	}

	languageCode := Language(tag.String())
	if !languageCode.IsSupported() {
		return "", fmt.Errorf("%w: %q", ErrUnsupportedLanguage, value)
	}

	return languageCode, nil
}

// IsSupported reports whether an embedded catalog exists for the language.
func (l Language) IsSupported() bool {
	_, ok := supportedLanguages[l]
	return ok
}

// New creates a localizer for a supported language with the default fallback.
func New(languageCode Language) (Localizer, error) {
	if !languageCode.IsSupported() {
		return Localizer{}, fmt.Errorf("%w: %q", ErrUnsupportedLanguage, languageCode)
	}

	return Localizer{
		localizer: i18n.NewLocalizer(bundle, string(languageCode), string(Default)),
	}, nil
}

// MustNew creates a localizer for a previously validated language.
func MustNew(languageCode Language) Localizer {
	localizer, err := New(languageCode)
	if err != nil {
		panic(err)
	}

	return localizer
}

// Text returns a translated message identified by a stable catalog key.
func (l Localizer) Text(id string, data any) string {
	return l.localizer.MustLocalize(&i18n.LocalizeConfig{
		MessageID:    id,
		TemplateData: data,
	})
}

func loadBundle() (*i18n.Bundle, map[Language]struct{}) {
	loaded := i18n.NewBundle(textlanguage.Russian)
	loaded.RegisterUnmarshalFunc("json", json.Unmarshal)

	filenames, err := fs.Glob(files, "locales/active.*.json")
	if err != nil {
		panic(err)
	}

	supported := make(map[Language]struct{}, len(filenames))
	for _, filename := range filenames {
		code := strings.TrimSuffix(strings.TrimPrefix(path.Base(filename), "active."), ".json")
		tag, err := textlanguage.Parse(code)
		if err != nil {
			panic(fmt.Errorf("parse localization catalog %q: %w", filename, err))
		}
		if _, err := loaded.LoadMessageFileFS(files, filename); err != nil {
			panic(err)
		}
		supported[Language(tag.String())] = struct{}{}
	}

	if _, ok := supported[Default]; !ok {
		panic("default localization catalog is missing")
	}

	return loaded, supported
}
