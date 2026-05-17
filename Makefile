BINARY   := trackerd
GOFLAGS  := -ldflags="-s -w" -trimpath
GOBIN    := $(HOME)/go/bin/go
GOPATH   := $(HOME)/gopath
GOROOT   := $(HOME)/go

# Pull modules directly from source — bypasses proxy.golang.org
# which is blocked or unreachable on many Kali installs
GOENV    := GOPATH=$(GOPATH) GOROOT=$(GOROOT) GOPROXY=direct GONOSUMCHECK=* GONOSUMDB=*

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
