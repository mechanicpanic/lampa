.PHONY: all tap lampa test

all: tap lampa

tap:
	swift build -c release --package-path tap
	mkdir -p bin
	# rm first: overwriting a signed binary in place makes macOS SIGKILL it
	rm -f bin/lampa-tap bin/libnowplaying.dylib
	cp tap/.build/release/lampa-tap bin/
	clang -dynamiclib -O2 -fobjc-arc -framework Foundation tap/nowplaying/nowplaying.m -o bin/libnowplaying.dylib

lampa:
	rm -f bin/lampa
	go build -o bin/lampa ./cmd/lampa

test:
	go test ./... -race
