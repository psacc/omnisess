## 1. Makefile Targets

- [ ] 1.1 Add `tag` target: validates `VERSION` is set, creates annotated git tag `$(VERSION)` with message "Release $(VERSION)", pushes tag to origin
- [ ] 1.2 Add `release` target: depends on `tag`, checks `gh` is installed, runs `gh release create $(VERSION) --generate-notes`, prints `plugin.json` sync reminder after success
- [ ] 1.3 Add `tag` and `release` to `.PHONY` declaration

## 2. Documentation

- [ ] 2.1 Create `docs/process/release.md`: sections for when-to-release, semver rules, step-by-step release instructions, and `plugin.json` sync procedure
- [ ] 2.2 Update `README.md`: add a "Releases" section with a link to `https://github.com/psacc/omnisess/releases`

## 3. First Release

- [ ] 3.1 Update `plugin.json` version field from `1.0.0` to `0.1.0`
- [ ] 3.2 Commit all artifacts from this change
- [ ] 3.3 Run `make tag VERSION=v0.1.0` to create and push the first release tag
- [ ] 3.4 Run `gh release create v0.1.0 --generate-notes --title "v0.1.0"` to publish the GitHub Release (or use `make release VERSION=v0.1.0`)

## 4. Verification

- [ ] 4.1 Confirm `git tag` lists `v0.1.0`
- [ ] 4.2 Confirm GitHub Releases page shows the v0.1.0 release
- [ ] 4.3 Confirm `go install github.com/psacc/omnisess@v0.1.0` resolves and builds
- [ ] 4.4 Confirm `make check` still passes after all file changes
