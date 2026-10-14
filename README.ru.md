# lampa ✿

[English](README.md)

Музыка с Mac → лампы Yeelight (music mode) и WiZ в реальном времени. TUI со спектром, долями и живыми плашками ламп.

## Сборка

```bash
make            # bin/lampa-tap (Swift) + bin/lampa (Go)
make test
```

Нужны macOS ≥ 14.4, Go, Swift. При первом запуске macOS спросит разрешение на запись звука для терминала.

## Запуск

```bash
bin/lampa discover --save   # найти лампы и записать в ~/.config/lampa/config.toml
bin/lampa                   # весь звук системы
bin/lampa --dry             # без ламп, только экран
bin/lampa --wav track.wav   # из файла
bin/lampa --lang en         # английский интерфейс (по умолчанию русский; t переключает)
```

Клавиши: `1–5` режим (Pulse, Spectrum, Chase, Ambient, Drop), `p` палитра, `←/→` чувствительность, `↑/↓` задержка света, `s` источник звука, `l` выбор ламп (вкл/выкл, запоминается), `d` поиск ламп, `t` язык, `space` пауза, `q` выход (лампы вернутся как были).

## Роли ламп

В конфиге `role = "bass" | "mid" | "high" | "lowmid" | "sub"` задаёт, за какой полосой лампа следит в режиме Spectrum.

## Yeelight

Включи «LAN Control» в приложении Yeelight. lampa сама переводит лампу в music mode (без лимита 60 команд/мин); если модель его не поддерживает, работает медленно и пишет об этом на плашке.
