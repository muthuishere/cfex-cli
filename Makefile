.PHONY: build test install
build:
	go build -o cfex ./cmd/cfex
# The suite is fail-closed: it refuses to run unless CFEX_CONFIG is under the temp dir.
test:
	go vet ./...
	CFEX_CONFIG=$$(mktemp -d)/config.yaml go test -count=1 ./...
install:
	go install ./cmd/cfex
