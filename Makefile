.PHONY: all test vet fmt validate build clean

all: build

build:
	go build -o mp4repair .

test:
	go test -count=1 ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

validate:
	bash scripts/validate_real.sh

clean:
	rm -f mp4repair output/mp4repair output/mkfixtures
