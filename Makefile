BINARY   := trackerd
GOFLAGS  := -ldflags="-s -w" -trimpath
GOBIN    := $(shell which go 2>/dev/null || echo $(HOME)/go/bin/go)

# GOROOT is intentionally NOT set — Go locates its own stdlib automatically.
# Setting it manually causes "not in std" errors on machines with different Go installs.
# GOPROXY=direct bypasses proxy.golang.org which is unreachable on many Kali installs.
GOENV    := GOPROXY=direct GONOSUMCHECK=* GONOSUMDB=*

.PHONY: build build-linux build-arm build-mac clean

build:
	$(GOENV) $(GOBIN) build $(GOFLAGS) -o $(BINARY) .

build-linux:
	$(GOENV) GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
		$(GOBIN) build $(GOFLAGS) -o $(BINARY)_linux_amd64 .

build-arm:
	$(GOENV) GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
		$(GOBIN) build $(GOFLAGS) -o $(BINARY)_linux_arm64 .

build-mac:
	$(GOENV) GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 \
		$(GOBIN) build $(GOFLAGS) -o $(BINARY)_darwin_arm64 .

clean:
	rm -f $(BINARY) $(BINARY)_linux_amd64 $(BINARY)_linux_arm64 $(BINARY)_darwin_arm64
