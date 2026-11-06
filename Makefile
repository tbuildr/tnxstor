BINARY := tnxs
PKG := ./cmd/tnxs

# CGO_ENABLED=0 produces a fully static binary that runs on any Linux
# host regardless of its C library (glibc/musl) — no interpreter or
# linker required at runtime.
.PHONY: build test vet tidy clean

build:
	CGO_ENABLED=0 go build -o $(BINARY) $(PKG)

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -f $(BINARY)
