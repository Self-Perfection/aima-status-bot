BINARY := aima-renew-watch-bot
PKG    := ./cmd/bot

.PHONY: build build-linux test vet clean

# Сборка под текущую платформу.
build:
	go build -o $(BINARY) $(PKG)

# Кросс-сборка под Linux/amd64 для деплоя на сервер.
# CGO_ENABLED=0 даёт полностью статический бинарь — не зависит от версии glibc.
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BINARY) $(PKG)

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f $(BINARY)
