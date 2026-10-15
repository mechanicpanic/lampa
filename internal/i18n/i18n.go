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
	"h hide sleeping":                      "h скрыть выключенные",
	"h show sleeping":                      "h показать выключенные",
	// help screen
	"click":                 "клик",
	"help":                  "помощь",
	"Modes":                 "Режимы",
	"Settings":              "Настройки",
	"Lamps":                 "Лампы",
	"Sound":                 "Звук",
	"Other":                 "Другое",
	"Config":                "Конфиг",
	"✿ Help":                "✿ Помощь",
	"↑↓ scroll · esc close": "↑↓ листать · esc закрыть",
	"Pulse — every lamp flashes on the beat; stacked lamps rise floor by floor like a fountain": "Pulse — все лампы вспыхивают на долю; лампы в группе — по этажам снизу вверх, как фонтанчик",
	"Spectrum — each lamp follows a band: bass, mids or highs (in a group: bass at the bottom)": "Spectrum — каждая лампа следит за своей полосой: бас, середина или верха (в группе бас внизу)",
	"Chase — the light jumps to the next lamp on every beat":                                    "Chase — свет перескакивает на следующую лампу на каждую долю",
	"Ambient — slow drift for background music; tempo sets the speed, loudness the colour":      "Ambient — медленный перелив для фона; темп задаёт скорость, громкость — насыщенность",
	"Drop — tension builds with the music, then a white flash on the drop":                      "Drop — свет копит напряжение вместе с музыкой, на дропе белая вспышка",
	"Meter — a volume meter: the track's average sits half-way, loud moments and kicks go up":   "Meter — столбик громкости: средний уровень трека на половине, громкие места и бочка — вверх",
	"palette — 11 colour palettes; the spectrum shows the current one":                          "палитра — 11 палитр; спектр окрашен в текущую",
	"sensitivity — how easily beats are caught, and how far Meter swings":                       "чувствительность — как легко ловятся удары и как сильно качается Meter",
	"delay — move the light earlier or later to match your speakers":                            "задержка — сдвинуть свет раньше или позже, чтобы совпадал с колонками",
	"lamps list: space switches a lamp on/off, h hides sleeping lamps from this screen":         "список ламп: space включает/выключает, h скрывает выключенные с главного экрана",
	"click a card to switch a lamp; in a group, click a floor bar or the group name":            "клик по карточке включает/выключает лампу; в группе — по полоске этажа или по имени группы",
	"find lamps on the network and add new ones to the config":                                  "найти лампы в сети и добавить новые в конфиг",
	"source — the whole system or one app (Spotify, a browser…)":                                "источник — вся система или одно приложение (Spotify, браузер…)",
	"pause — lamps keep their last colour":                                                      "пауза — лампы держат последний цвет",
	"language — Russian / English":                                                              "язык — русский / английский",
	"quit — every lamp goes back to how it was":                                                 "выход — все лампы возвращаются как были",
	"help — this screen": "помощь — этот экран",
	"~/.config/lampa/config.toml — edit it with lampa closed (it saves on exit)":                        "~/.config/lampa/config.toml — правь при закрытой lampa (она сохраняет его при выходе)",
	"name = \"kitchen\" — what a card is called":                                                        "name = \"кухня\" — как называется карточка",
	"role = \"bass\" | \"mid\" | \"high\" | \"lowmid\" | \"sub\" — the band a lamp follows in Spectrum": "role = \"bass\" | \"mid\" | \"high\" | \"lowmid\" | \"sub\" — какую полосу лампа слушает в Spectrum",
	"group = \"ikea\", level = 1 (bottom), 2, 3… — lamps stacked in one fixture":                        "group = \"ikea\", level = 1 (низ), 2, 3… — лампы этажами в одном светильнике",
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
