BINARY=terraform-provider-incusos
VERSION?=dev

.PHONY: build test testacc docs lint vulncheck fmt vet clean

build:
	go build -ldflags="-X main.version=$(VERSION)" -o $(BINARY) .

test:
	go test -v ./...

testacc:
	TF_ACC=1 go test -v -count=1 -timeout 10m ./...

docs:
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest generate --provider-name incusos

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

fmt:
	gofmt -s -w .

vet:
	go vet ./...

clean:
	rm -f $(BINARY)
