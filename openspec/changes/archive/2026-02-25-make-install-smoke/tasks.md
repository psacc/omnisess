## 1. Makefile Targets

- [x] 1.1 Add `install` target to Makefile: runs `go install .`
- [x] 1.2 Add `smoke` target to Makefile: checks PATH for omnisess, runs `omnisess list --limit=1`, fails with clear PATH hint if not found
- [x] 1.3 Add `install` and `smoke` to the `.PHONY` declaration

## 2. Documentation Updates

- [x] 2.1 Update `docs/process/git-workflow.md` step 4 (Smoke) in The Full Flow to reference `make smoke`
- [x] 2.2 Update `docs/process/git-workflow.md` pre-merge checklist item to use `make smoke`
- [x] 2.3 Update `README.md` build-from-source block to include `make install && make smoke`

## 3. Verification

- [x] 3.1 Run `make install` and confirm binary lands in `~/go/bin/omnisess`
- [x] 3.2 Run `make smoke` and confirm it exits 0 and prints output
