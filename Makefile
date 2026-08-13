.PHONY: build test vet fmt
build:
	go build -o msg ./cmd/msg
test:
	go test ./...
vet:
	go vet ./...
fmt:
	gofmt -w cmd internal
