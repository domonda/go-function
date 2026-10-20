# TODOS

Problems found by the reviews of the v1.0.0 release that predate it.

## Generator (cmd/gen-func-wrappers)

### Generated imports can collide with package-level names

**What:** Give the imports of generated code an alternative name when the package declares the name at package level.

**Why:** A package-level declaration like `var reflect = 1` in any file of the package makes the generated code fail to compile with "reflect already declared through import", while the generator exits with 0.

**Context:** `targetFile.nameConflicts` in `internal/gen/imports.go` only checks the imports of the target file and the identifiers of the generated code. It also has to check the package-scope names of all files of the package for `context`, `fmt`, `json`, `reflect` and the name of go-function, and use an alias like for other conflicts.

**Effort:** S
**Priority:** P2
**Depends on:** None

### Cache resolved packages across files

**What:** Share the resolved import paths and package names of one generator run between the files it rewrites.

**Why:** Every rewritten file locates its imports with the go command again, which makes recursive runs over large module trees slow.

**Context:** The per-file resolution in `internal/gen/imports.go` was deferred from the v1.0.0 review as a performance improvement without correctness impact.

**Effort:** M
**Priority:** P3
**Depends on:** None

## Infrastructure

### tag-release.sh doesn't check that the version is new

**What:** Reject versions that are not above the latest release tag.

**Why:** The script only rejects versions whose tags already exist, so a typo like `v0.9.0` after `v1.0.0` would publish a lower version.

**Context:** `tag-release.sh` has `git tag -l --sort=-v:refname` for listing tags; compare the version with the latest tag of the root module before creating tags.

**Effort:** S
**Priority:** P3
**Depends on:** None

## Completed

### VoidFuncWrapper and ErrorFuncWrapper ignore their arguments

They return errors for passed arguments or strings and for JSON that is no object, like `ReflectWrapper`.

**Completed:** v1.0.0 (2026-10-19)
