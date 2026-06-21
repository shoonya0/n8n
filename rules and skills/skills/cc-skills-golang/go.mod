// This is a vendored, read-only copy of samber/cc-skills-golang — skill
// documentation plus illustrative Go example snippets (under skills/*/assets/).
//
// It is declared as a SEPARATE Go module on purpose: the parent n8n
// module lives at the repo root, and its directory path contains a space
// ("rules and skills/"), which is not a legal Go import path. Without this
// nested module boundary, `go build ./...`, `go vet ./...`, `go test ./...`
// and `go mod tidy` run from the repo root fail with
//   malformed import path "...rules and skills/...": invalid char ' '
// Go does not descend into directories that are themselves module roots, so
// this file makes the toolchain skip the entire vendored tree. The example
// snippets here are never compiled as part of the backend.
module local/cc-skills-golang-reference

go 1.25
