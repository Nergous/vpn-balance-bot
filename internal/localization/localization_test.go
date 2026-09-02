package localization

import (
	"errors"
	"testing"
)

func TestLocalizerUsesCatalogsAndTemplateData(t *testing.T) {
	if got := MustNew(Russian).Text("StartBound", nil); got != "Профиль привязан. Используйте /status." {
		t.Fatalf("Russian message = %q", got)
	}
	if got := MustNew(English).Text("StatusDebt", map[string]any{"Amount": 25000, "Currency": "RUB"}); got != "Debt: 25000 RUB" {
		t.Fatalf("English template = %q", got)
	}
	if _, err := New("de"); !errors.Is(err, ErrUnsupportedLanguage) {
		t.Fatalf("New(de) error = %v, want %v", err, ErrUnsupportedLanguage)
	}
}

func TestParseLanguageUsesEmbeddedCatalogRegistry(t *testing.T) {
	if Default != Russian {
		t.Fatalf("Default = %q, want %q", Default, Russian)
	}
	if !Russian.IsSupported() || !English.IsSupported() {
		t.Fatal("Russian and English catalogs must remain supported")
	}

	got, err := Parse(" EN ")
	if err != nil || got != English {
		t.Fatalf("Parse(EN) = %q, %v", got, err)
	}
	if _, err := Parse("de"); !errors.Is(err, ErrUnsupportedLanguage) {
		t.Fatalf("Parse(de) error = %v, want %v", err, ErrUnsupportedLanguage)
	}
}
