PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin
DATADIR ?= $(PREFIX)/share

.PHONY: build run test check install install-system uninstall clean
build:
	go build -trimpath -o bin/messenger-hub ./cmd/messenger-hub
run: build
	./bin/messenger-hub
test:
	go test ./...
check:
	go vet ./...
	go test ./...
install: build
	install -Dm755 bin/messenger-hub "$(DESTDIR)$(BINDIR)/messenger-hub"
	install -Dm644 packaging/io.github.messengerhub.MessengerHub.desktop "$(DESTDIR)$(DATADIR)/applications/io.github.messengerhub.MessengerHub.desktop"
	install -Dm644 packaging/io.github.messengerhub.MessengerHub.svg "$(DESTDIR)$(DATADIR)/icons/hicolor/scalable/apps/io.github.messengerhub.MessengerHub.svg"
install-system:
	./scripts/install.sh --system
uninstall:
	rm -f "$(DESTDIR)$(BINDIR)/messenger-hub" "$(DESTDIR)$(DATADIR)/applications/io.github.messengerhub.MessengerHub.desktop" "$(DESTDIR)$(DATADIR)/icons/hicolor/scalable/apps/io.github.messengerhub.MessengerHub.svg"
clean:
	rm -rf bin
