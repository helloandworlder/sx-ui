package locale

import (
	"testing"
	"testing/fstest"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/text/language"
)

func TestParseTranslationFilesSkipsHiddenArtifacts(t *testing.T) {
	bundle := i18n.NewBundle(language.MustParse("en-US"))
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)

	translationFS := fstest.MapFS{
		"translation/translate.en_US.toml": {
			Data: []byte(`[pages.index]
welcome = "Welcome"`),
		},
		"translation/._translate.en_US.toml": {
			Data: []byte{0x00, 0x01, 0x02, 0x03},
		},
	}

	if err := parseTranslationFiles(translationFS, bundle); err != nil {
		t.Fatalf("parseTranslationFiles should ignore hidden artifacts: %v", err)
	}

	localizer := i18n.NewLocalizer(bundle, "en-US")
	got, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "pages.index.welcome"})
	if err != nil {
		t.Fatalf("expected bundled translation to remain available: %v", err)
	}
	if got != "Welcome" {
		t.Fatalf("unexpected translation: got %q", got)
	}
}
