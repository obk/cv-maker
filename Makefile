VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.PHONY: build install test preview dist clean

build:
	go build -ldflags "$(LDFLAGS)" -o bin/cvmaker ./cmd/cvmaker

# Installs into $GOBIN (default: $(go env GOPATH)/bin).
install:
	go install -ldflags "$(LDFLAGS)" ./cmd/cvmaker

test:
	go vet ./...
	go test ./...

# Needs pdftoppm from poppler-utils.
preview: build
	./bin/cvmaker build examples/cv.md -o bin/preview.pdf
	pdftoppm -png -r 110 -singlefile bin/preview.pdf docs/preview

# Release archives for every platform in PLATFORMS, plus checksums.
dist:
	rm -rf dist && mkdir -p dist
	set -e; for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; name=cvmaker_$(VERSION)_$${os}_$${arch}; \
		ext=; [ $$os = windows ] && ext=.exe; \
		mkdir -p dist/$$name; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$$name/cvmaker$$ext ./cmd/cvmaker; \
		cp README.md LICENSE dist/$$name/; \
		if [ $$os = windows ]; then (cd dist && zip -qr $$name.zip $$name); \
		else tar -C dist -czf dist/$$name.tar.gz $$name; fi; \
		rm -rf dist/$$name; \
	done
	cd dist && sha256sum * > checksums.txt

clean:
	rm -rf bin dist
