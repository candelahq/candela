## Summary
<!-- What does this PR do? Why is it needed? Link to the issue it addresses. -->

Closes #

## Type of Change
<!-- Check the one that applies. -->

- [ ] 🐛 Bug fix (non-breaking change that fixes an issue)
- [ ] ✨ New feature (non-breaking change that adds functionality)
- [ ] 💥 Breaking change (fix or feature that would cause existing functionality to change)
- [ ] 📝 Documentation update
- [ ] ♻️ Refactor (no functional changes)
- [ ] 🔧 Chore (build, CI, dependencies, tooling)

## Changes
<!-- List the key changes made. Be specific. -->

-

## Testing
<!-- How did you verify this works? Check all that apply. -->

- [ ] Go tests pass (`go test ./...`)
- [ ] Go lint passes (`golangci-lint run`)
- [ ] UI builds (`cd ui && pnpm run build`)
- [ ] Playwright E2E tests pass (`cd ui && pnpm run test:e2e`)
- [ ] Functional tests pass (`./test/functional/run.sh --go`)
- [ ] Rust tests pass (`cargo test`) — _if Rust code changed_
- [ ] Manual verification (describe below)

<!-- Describe any manual testing you did: -->

## Screenshots / Recordings
<!-- If this is a UI change, add before/after screenshots or a screen recording. Delete this section if not applicable. -->

## Migration / Deployment Impact
<!-- Does this PR require any of the following? Delete this section if not applicable. -->

- [ ] Database schema migration
- [ ] Config changes (update `config.example.yaml`)
- [ ] Proto regeneration (`buf generate`)
- [ ] Infrastructure changes (Terraform / Helm)
- [ ] Dependency updates that affect production builds

## Related PRs
<!-- Link any related PRs across repos (e.g., proto changes in candela-protos). Delete if not applicable. -->

-

## Checklist

- [ ] I've read [CONTRIBUTING.md](../CONTRIBUTING.md)
- [ ] My commit messages follow [Conventional Commits](https://www.conventionalcommits.org/)
- [ ] Breaking changes are documented
- [ ] New config options are added to `config.example.yaml`
