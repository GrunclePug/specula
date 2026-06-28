include config.mk

PREFIX ?= /usr/local
DESTDIR ?=

.PHONY: all build clean run test install

all: build

build:
	@mkdir -p bin
	go build -ldflags "-X main.Version=$(VERSION)" -o bin/specula ./cmd/specula

clean:
	rm -rf bin/

run: build
	./bin/specula

test:
	go test -v ./...

install: build
	install -d $(DESTDIR)$(PREFIX)/bin
	install -m 755 bin/specula $(DESTDIR)$(PREFIX)/bin/specula
