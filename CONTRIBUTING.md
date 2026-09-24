# Contributing

Thanks for helping improve the Pathly Kubernetes operator.

## Prerequisites

- Go (see `go.mod`)
- Access to a Kubernetes cluster for smoke tests (optional)
- A Pathly API token from [pathlyhq.com](https://pathlyhq.com) ([developers](https://pathlyhq.com/en/developers))

## Develop

```bash
make tidy
make test    # go test ./internal/... — keep 100% coverage
make build
```

## Guidelines

- Prefer Secret refs for credentials; never put tokens in example manifests.
- Keep `PathlyScenario` HTTP-only (`type: http`). Do not add browser-step fields to the CRD.
- Docs and examples should link to [pathlyhq.com](https://pathlyhq.com) only.
- Open a PR with a clear description and a linked issue when applicable.

## License

Contributions are accepted under the Apache-2.0 license (see [LICENSE](LICENSE)).
