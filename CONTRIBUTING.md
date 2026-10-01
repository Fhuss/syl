# Contributing to Syl

Thank you for your interest in contributing to **Syl**! We welcome community contributions, bug reports, and suggestions.

To ensure high quality, stability, and clean history, please follow these guidelines.

---

## Code of Conduct & Contribution Philosophy

- **Stability First**: Syl runs continuously in developer split terminals alongside active coding agents. It must never freeze, leak memory, or crash unexpectedly.
- **Maintain Documentation Integrity**: Preserve existing comments, docstrings, and test suites.
- **Keep Diffs Minimal**: Avoid large indiscriminate reformatting. Focus PRs on one specific improvement or fix.

---

## Development Setup

### Prerequisites
- [Go 1.23+](https://golang.org)
- A terminal multiplexer (cmux, tmux, or terminal tabs)

### Building and Running Locally
```bash
# Clone the repository
git clone https://github.com/Fhuss/syl.git
cd syl

# Run tests
go test -count=1 ./...

# Build binary
go build -o bin/syl ./cmd/syl

# Run interactive demo
./bin/syl demo
```

---

## Submitting Pull Requests

1. **Create a Feature Branch**:
   ```bash
   git checkout -b feature/my-cool-improvement
   ```

2. **Commit Guidelines**:
   We follow [Conventional Commits](https://www.conventionalcommits.org/):
   - `feat:` for new capabilities or flags
   - `fix:` for bug fixes
   - `perf:` for performance optimizations
   - `docs:` for documentation updates
   - `refactor:` for internal improvements without behavioral change

3. **Verify Everything Passes**:
   ```bash
   go test -v ./...
   ```
   All tests must pass before opening a PR.

4. **Open a PR**:
   - Provide a clear description of the problem and the solution.
   - PRs must pass the automated GitHub Actions CI tests before they can be merged.
   - All PRs are reviewed and merged by repository maintainers.
