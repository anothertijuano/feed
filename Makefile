# feed. — build & run targets

BIN     := feed
LDFLAGS := -s -w

.PHONY: build test vet run clean gen-vapid

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) .

# Cross-compile for common self-hosting targets.
linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN)-linux-amd64 .

linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN)-linux-arm64 .

test:
	go test ./...

vet:
	go vet ./...

run:
	go run .

gen-vapid:
	go run . -gen-vapid

clean:
	rm -f $(BIN) $(BIN)-linux-amd64 $(BIN)-linux-arm64
