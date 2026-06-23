# Contributing to ProxyPool

Thanks for your interest in contributing! This project is small and focused — contributions are welcome but should follow these guidelines.

## Quick Rules

1. **No new dependencies** without discussion. Current dep count: 1 (`gopkg.in/yaml.v3`). This is a feature, not a bug.
2. **No cgo.** Everything must stay `CGO_ENABLED=0`. If your change needs cgo, it won't be accepted.
3. **`gofmt` + `go vet`** must pass with zero warnings.
4. **Comment every exported identifier.**
5. **Keep the binary small.** Target is <10 MB. If your change adds 2 MB, explain why.
6. **Test on at least 2 platforms** (e.g. macOS + Linux). We support 7 target combinations.

## Development Setup

```bash
# Clone
git clone https://github.com/jeff/proxypool.git
cd proxypool

# Build for your host
./scripts/build.sh

# Run from source
go run ./cmd/proxypool -config configs/config.example.yaml

# Run tests (requires Python 3 for echo servers)
python3 scripts/test_echo_servers.py &
go test ./...
```

## Pull Request Checklist

- [ ] `gofmt -w .` passes
- [ ] `go vet ./...` passes
- [ ] `go build ./cmd/proxypool` succeeds
- [ ] Existing tests still pass
- [ ] New features have tests
- [ ] No hardcoded credentials, IPs, or usernames
- [ ] README updated if behavior changed
- [ ] Commit messages follow `type: subject` convention (e.g. `feat: add udp support`)

## Commit Convention

```
type: concise subject line

Optional body explaining why.
```

Types: `feat`, `fix`, `refactor`, `docs`, `chore`, `test`

## Reporting Issues

- Include OS, architecture, and proxypool version (`proxypool -version`)
- Include your config (redact credentials!)
- Include relevant log output
- If it's a crash, include the full stack trace

## License

By contributing, you agree that your contributions are licensed under the MIT license.
