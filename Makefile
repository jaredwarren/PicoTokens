.PHONY: run-server build-server build-pico clean help

# Default target displays help
help:
	@echo "Available commands:"
	@echo "  make run-server   - Run the Go stats server on your Mac"
	@echo "  make build-server - Build the Go stats server binary"
	@echo "  make build-pico   - Compile the Pico W client C/C++ firmware (requires Pico SDK & ARM Toolchain)"
	@echo "  make clean        - Clean built binaries and Pico build folder"

# Run the Go Server locally
run-server:
	cd server && go run main.go render.go

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
