# Building AI Chat app in Go Lang, Python, SQLite 

## Testing during development

Run the relevant tests while iterating. For example:

```sh
go test ./internal/tui/... -run 'TestModel_BrowseThoughtsView'
go test ./internal/tui/thoughts -run 'TestModel_InputIntegrity'
go test -tags=integration ./integration -run 'TestThoughtCountWorkflow_SQLite'
```

Add `-count=1` when you need a fresh execution rather than Go's test cache.
Run the complete checks at the usual boundaries:

- `make quick` before committing (also run by the pre-commit hook).
- `make check` before pushing (also run by the pre-push hook).
- `make ci` for complete verification, including the race detector.

`make ci` already includes `make check`; running both consecutively repeats the
fresh unit tests, integration tests, and build. Keep the required hooks enabled.
For timing comparisons, run tests without other test/build processes competing
for resources, and measure compilation separately from test execution.
