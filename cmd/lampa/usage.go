package main

import (
	"fmt"
	"io"
	"strings"

	"lampa/internal/config"
	"lampa/internal/i18n"
)

// usageEN is the English help; Russian lives in usageRU. Shown by -h,
// --help and `lampa help`.
const usageEN = `lampa — your Mac's music → Yeelight and WiZ bulbs, in real time.

Usage:
  lampa [flags]              open the TUI and drive your lamps
  lampa discover [--save]    find lamps on the network (--save adds them to the config)
  lampa help                 this text

Flags:
  --dry            no real lamps: virtual lamps on screen only
  --wav FILE       play a WAV file instead of system audio
  --lang ru|en     interface language (default: from the config, else Russian)
  --config PATH    config file (default ~/.config/lampa/config.toml)
  --tap PATH       path to the lampa-tap audio helper (found automatically)

Keys (press ? inside lampa for the full help):
  1–6   mode: Pulse, Spectrum, Chase, Ambient, Drop, Meter
  p     palette        ←→ sensitivity      ↑↓ light delay
  s     audio source   l  lamps (space on/off, h hide sleeping)
  d     find lamps     ␣  pause      t language      q quit
  click a lamp card to switch it on/off

Config (~/.config/lampa/config.toml — edit with lampa closed):
  name = "kitchen"                         what the card is called
  role = "bass" | "mid" | "high" | ...     the band a lamp follows in Spectrum
  group = "ikea", level = 1 (bottom) …     lamps stacked in one fixture
  disabled = true                          lamp switched off in lampa

Examples:
  lampa discover --save      first run: find and remember your lamps
  lampa                      whole-system audio
  lampa --dry                try it without lamps
  lampa --lang en            English interface

macOS asks once to allow audio recording for your terminal.
Update: brew upgrade lampa · Source: https://github.com/mechanicpanic/lampa
`

const usageRU = `lampa — музыка с Mac → лампы Yeelight и WiZ в реальном времени.

Запуск:
  lampa [флаги]              открыть интерфейс и управлять лампами
  lampa discover [--save]    найти лампы в сети (--save добавит их в конфиг)
  lampa help                 этот текст

Флаги:
  --dry            без настоящих ламп: виртуальные лампы только на экране
  --wav ФАЙЛ       играть WAV-файл вместо звука системы
  --lang ru|en     язык интерфейса (по умолчанию из конфига, иначе русский)
  --config ПУТЬ    файл конфига (по умолчанию ~/.config/lampa/config.toml)
  --tap ПУТЬ       путь к помощнику захвата звука lampa-tap (находится сам)

Клавиши (полная помощь внутри lampa — клавиша ?):
  1–6   режим: Pulse, Spectrum, Chase, Ambient, Drop, Meter
  p     палитра        ←→ чувствительность ↑↓ задержка света
  s     источник звука l  лампы (space вкл/выкл, h скрыть выключенные)
  d     поиск ламп     ␣  пауза      t язык          q выход
  клик по карточке лампы включает/выключает её

Конфиг (~/.config/lampa/config.toml — правь при закрытой lampa):
  name = "кухня"                           как называется карточка
  role = "bass" | "mid" | "high" | ...     какую полосу лампа слушает в Spectrum
  group = "ikea", level = 1 (низ) …        лампы этажами в одном светильнике
  disabled = true                          лампа выключена в lampa

Примеры:
  lampa discover --save      первый запуск: найти и запомнить лампы
  lampa                      звук всей системы
  lampa --dry                попробовать без ламп
  lampa --lang en            английский интерфейс

macOS один раз спросит разрешение на запись звука для терминала.
Обновить: brew upgrade lampa · Исходники: https://github.com/mechanicpanic/lampa
`

func printUsage(w io.Writer) {
	if i18n.Current() == i18n.RU {
		fmt.Fprint(w, usageRU)
		return
	}
	fmt.Fprint(w, usageEN)
}

// earlyLang picks the help language before flags are parsed: --lang wins,
// then the config, then Russian.
func earlyLang(args []string) i18n.Lang {
	for i, a := range args {
		if v, ok := strings.CutPrefix(a, "--lang="); ok {
			return i18n.Parse(v)
		}
		if (a == "--lang" || a == "-lang") && i+1 < len(args) {
			return i18n.Parse(args[i+1])
		}
	}
	cfg, _ := config.Load(config.DefaultPath())
	return i18n.Parse(cfg.Lang)
}

// wantsHelp: "help", -h or --help anywhere, except as a flag's value.
func wantsHelp(args []string) bool {
	for i, a := range args {
		if i > 0 && strings.HasPrefix(args[i-1], "-") && !strings.Contains(args[i-1], "=") && args[i-1] != "-h" && args[i-1] != "--help" && args[i-1] != "--dry" && args[i-1] != "-dry" {
			continue // value of the previous flag, e.g. --wav help.wav
		}
		if a == "help" || a == "-h" || a == "--help" || a == "-help" {
			return true
		}
	}
	return false
}
