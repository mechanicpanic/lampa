.PHONY: all tap lampa test

all: tap lampa

tap:
	swift build -c release --package-path tap
	mkdir -p bin
	cp tap/.build/release/lampa-tap bin/

lampa:
	go build -o bin/lampa ./cmd/lampa

test:
	go test ./... -race
