BINARY := aima-renew-watch-bot
PKG    := ./cmd/bot

.PHONY: build build-linux test vet clean

# Сборка под текущую платформу.
build:
	go build -o $(BINARY) $(PKG)

# Кросс-сборка под Linux/amd64 для деплоя на сервер.
build-linux:
	GOOS=linux GOARCH=amd64 go build -o $(BINARY) $(PKG)

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f $(BINARY)
