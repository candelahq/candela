# 🤝 Contributing to Candela

We are in the early stages of building a production-grade LLM observability platform. We welcome all contributions!

## 🚀 Getting Started

1.  **Clone the Repo**: `git clone https://github.com/candelahq/candela`
2.  **Enter the Dev Environment**: `nix develop`
3.  **Run the Tests**: `go test ./...`
4.  **Create a Branch**: `git checkout -b feat/my-new-feature`

## 🛠️ Development Workflow

### 📋 Code Style
- **Go**: We use `golangci-lint`. Ensure yours passes before opening a PR.
- **Protobuf**: Always generate code via `buf generate` in the `proto/` directory.
- **Commits**: We prefer [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/).

### 🧪 Testing
- **Unit Tests**: Place in the same package as your code (e.g., `pkg/proxy/proxy_test.go`).
- **Integration Tests**: We use the Nix dev shell to run database-backed tests.
- **New Features**: Every new feature **must** include tests.

## 🐛 Reporting Issues

Use the GitHub Issue tracker to report bugs or request features. Please include:
- A clear description of the issue.
- Steps to reproduce (if it's a bug).
- Your environment (Go version, OS, etc.).

## 📬 Submitting Pull Requests

1.  Keep PRs focused on a single change.
2.  Include a clear description of **what** changed and **why**.
3.  Ensure all tests and linting pass.
4.  Update documentation if you change the API or configuration.

## ⚖️ License

By contributing, you agree that your contributions will be licensed under the **Apache License 2.0**.
