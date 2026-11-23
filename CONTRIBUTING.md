# Contributing

Thank you for your interest in contributing to `terraform-provider-incusos`!

## Requirements

- Go (see `go.mod`)
- Terraform CLI (for running acceptance tests against the in-memory mock server)

## Development Workflow

```bash
# Format code
make fmt

# Run static analysis
make vet

# Run unit and acceptance tests
make test

# Build the provider binary
make build

# Regenerate Terraform Registry documentation
make docs
```

## Pull Requests

1. Fork the repository and create a feature branch from `main`.
2. Ensure all tests pass (`make test`) and code is formatted (`make fmt`).
3. If you add or modify resource/data source schemas, regenerate `docs/` with `make docs`.
4. Open a Pull Request describing the motivation and changes.
