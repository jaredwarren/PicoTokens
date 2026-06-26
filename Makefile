.PHONY: run-server build-server build-pico clean help kill-server

PORT ?= 8296

# Default target displays help
help:
	@echo "Available commands:"
	@echo "  make run-server   - Run the Go stats server on your Mac"
	@echo "  make kill-server  - Stop the Go stats server running on port $(PORT)"
	@echo "  make build-server - Build the Go stats server binary"
	@echo "  make build-pico   - Compile the Pico W client C/C++ firmware (requires Pico SDK & ARM Toolchain)"
	@echo "  make clean        - Clean built binaries and Pico build folder"

# Run the Go Server locally
run-server:
	cd server && go run main.go render.go

# Stop the Go Server
kill-server:
	@echo "Stopping server on port $(PORT)..."
	@lsof -t -i :$(PORT) | xargs kill -9 2>/dev/null || true

# Test the local Claude CLI usage poller and parser
test-cli:
	cd server && go run main.go render.go -test-cli

# Build the Go Server binary
build-server:
	cd server && go build -o server_bin main.go render.go

# Build the Pico W C/C++ Client
build-pico:
	mkdir -p pico/build
	cd pico/build && cmake .. && $(MAKE)

# Clean up build artifacts
clean:
	rm -f server/server_bin
	rm -rf pico/build
