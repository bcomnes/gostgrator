# Agent Guidelines

- In `.md` files, write Markdown prose with one sentence per line. Use a single newline between sentences within a paragraph and a blank line between paragraphs.
- In GitHub issue, pull request, and discussion descriptions or comments, keep each prose paragraph on one continuous line. Do not insert newlines between sentences in the same paragraph; GitHub renders them as visible line breaks. Use blank lines only to separate paragraphs, and preserve line breaks required by lists and code blocks.
- Format changed Go files with `gofmt` before finishing.
- Prefer the Go standard library unless a dependency clearly improves the implementation.
- Run `go mod tidy` only when adding, removing, or changing module dependencies.
- Keep `go.mod` and `go.sum` changes intentional and review them before finishing.
- Use the existing `Makefile` targets when they fit the task.
- Prefer `make test` or `go test ./...` for validation after code changes.
- Update exported identifiers' doc comments when changing public API behavior.
- Preserve PostgreSQL and SQLite behavior when changing shared migration code.
- When handling PR review comments, validate that each comment is correct before making changes; maintainer comments are almost always valid, but review bot comments may be wrong, and after addressing a comment, always reply with what was done.
