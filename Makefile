build:
	go build -o controller

test:
	go test -race -v ./...

vet:
	go vet ./...

.PHONY: build test vet