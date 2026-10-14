package tui

import (
	"os"
	"testing"

	"lampa/internal/i18n"
)

// Most tests assert English text; language tests switch explicitly.
func TestMain(m *testing.M) {
	i18n.Set(i18n.EN)
	os.Exit(m.Run())
}
