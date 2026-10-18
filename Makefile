BINARY := vibetutor
PKG := ./cmd/vibetutor

.PHONY: build run test race vet fmt clean docker

build:
	go build -o bin/$(BINARY) $(PKG)

run:
	go run $(PKG) -config config.txt

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

clean:
	rm -rf bin

docker:
	docker build -t $(BINARY) .
