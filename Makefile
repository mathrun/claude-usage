BINARY     := claude-usage
TARGET_DIR ?= /usr/local/bin
GOFLAGS    ?= -trimpath
LDFLAGS    ?= -s -w

# Absolute path of the built binary; the installed command is a symlink to it,
# so a later `make build` updates the installed command in place.
BIN_PATH   := $(abspath $(BINARY))

# Use sudo only when the target directory is not writable.
SUDO := $(shell test -w $(TARGET_DIR) || echo sudo)

.PHONY: all build install uninstall run test fmt vet lint clean

all: build

build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/$(BINARY)

install: build
	@echo "→ Linking $(BIN_PATH) to $(TARGET_DIR)/$(BINARY)"
	@$(SUDO) ln -sf "$(BIN_PATH)" "$(TARGET_DIR)/$(BINARY)"

uninstall:
	@if [ -L "$(TARGET_DIR)/$(BINARY)" ]; then \
		$(SUDO) rm -f "$(TARGET_DIR)/$(BINARY)"; \
		echo "→ Removed link $(TARGET_DIR)/$(BINARY)"; \
	fi

run: build
	./$(BINARY)

test:
	go test ./...

fmt:
	gofmt -l -w .

vet:
	go vet ./...

lint: fmt vet

clean:
	rm -f $(BINARY)
	rm -rf dist
