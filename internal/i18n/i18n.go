// Package i18n translates the UI. English text is the key (gettext style);
// Russian is looked up, anything unknown (app names, raw errors) passes through.
package i18n

import (
	"strings"
	"sync/atomic"
)

type Lang string

const (
	RU Lang = "ru"
	EN Lang = "en"
)

var current atomic.Value // Lang

func init() { current.Store(RU) }

func Set(l Lang)    { current.Store(l) }
func Current() Lang { return current.Load().(Lang) }
func Parse(s string) Lang {
	if strings.EqualFold(s, "en") {
		return EN
	}
	return RU
}

// Toggle switches RU ↔ EN and returns the new language.
func Toggle() Lang {
	l := RU
	if Current() == RU {
		l = EN
	}
	Set(l)
	return l
}

func T(en string) string {
	if Current() == RU {
		if s, ok := ru[en]; ok {
			return s
		}
	}
	return en
}

var ru = map[string]string{
	// main screen
	"window too small ✿ stretch me":          "окно маленькое ✿ растяни меня",
	"no lamps yet — press d and I'll look ✿": "ламп пока нет — нажми d, и я поищу ✿",
	"💤 sleeping":                   "💤 выключена",
	"✕ lost":                       "✕ потеряна",
	"… looking":                    "… ищу",
	"palette":                      "палитра",
	"sens %.2f":                    "чувств. %.2f",
	"delay %+d ms":                 "задержка %+d мс",
	"%+d ms":                       "%+d мс",
	"⏸ paused":                     "⏸ пауза",
	"no sound — play some music ♪": "нет звука — включи музыку ♪",
	"whole system":                 "вся система",
	// help
	"mode":   "режим",
	"sens":   "чувств.",
	"delay":  "задержка",
	"source": "источник",
	"lamps":  "лампы",
	"find":   "поиск",
	"pause":  "пауза",
	"quit":   "выход",
	"lang":   "язык",
	// pickers
	"♪ Audio source":                       "♪ Источник звука",
	"↑↓ choose · enter ok · esc cancel":    "↑↓ выбрать · enter ок · esc отмена",
	"✿ Lamps":                              "✿ Лампы",
	"no lamps — close this and press d":    "ламп нет — закрой и нажми d",
	"↑↓ choose · space on/off · esc close": "↑↓ выбрать · space вкл/выкл · esc закрыть",
	// status messages
	"sources: ":           "источники: ",
	"discovery: ":         "поиск: ",
	"new lamps found: %d": "найдено новых ламп: %d",
	"looking for lamps…":  "ищу лампы…",
	// lamp notes from drivers
	"waiting for reply":       "ждём ответа",
	"no reply":                "нет ответа",
	"no music mode: ≈0.5 fps": "без music mode: ≈0,5 кадра/с",
	// CLI
	"no lamps found (WiZ: is local control on? Yeelight: enable LAN Control in the app)": "ламп не найдено (WiZ: включено ли локальное управление? Yeelight: LAN Control в приложении)",
	" music mode: no":   " music mode: нет",
	" music mode: yes":  " music mode: да",
	"added to %s: %d\n": "добавлено в %s: %d\n",
}
