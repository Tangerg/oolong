# Pull request

## Outcome

Describe the caller-visible result and the problem it resolves.

## Design

State the owning type, output lifetime, dependency direction, and any rejected
alternative. Include benchmark evidence for performance claims.

## Verification

- [ ] Added or updated an external-package test for public behavior
- [ ] Added a changelog entry for caller-visible or breaking changes
- [ ] Kept application vocabulary out of framework packages
- [ ] Ran `gofumpt`, `shfmt`, and every module's tests
- [ ] Ran race, lint, vulnerability, Markdown, and standalone module gates when relevant
- [ ] Updated English and Chinese documentation together when they describe the same contract
