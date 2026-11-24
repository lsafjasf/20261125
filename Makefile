.PHONY: test vet build acceptance demo frames

test:
	go test -count=1 ./...

vet:
	go vet ./...

build:
	go build -o colorgraph .

frames:
	go run ./cmd/genframes frames/input

demo: build
	./colorgraph

acceptance:
	bash acceptance.sh
