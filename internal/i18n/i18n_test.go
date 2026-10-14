package i18n

import "testing"

func TestTranslate(t *testing.T) {
	defer Set(RU)
	Set(EN)
	if got := T("palette"); got != "palette" {
		t.Fatalf("en: %q", got)
	}
	Set(RU)
	if got := T("palette"); got != "палитра" {
		t.Fatalf("ru: %q", got)
	}
	if got := T("Spotify"); got != "Spotify" {
		t.Fatalf("unknown text must pass through, got %q", got)
	}
}

func TestParse(t *testing.T) {
	for in, want := range map[string]Lang{"en": EN, "EN": EN, "ru": RU, "": RU, "fr": RU} {
		if got := Parse(in); got != want {
			t.Fatalf("Parse(%q) = %q", in, got)
		}
	}
}

func TestEveryRussianEntryIsNonEmpty(t *testing.T) {
	for en, ru := range ru {
		if ru == "" {
			t.Fatalf("empty translation for %q", en)
		}
	}
}
