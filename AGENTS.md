# AGENTS.md

## Project Context

This is a Go + SQLite application for capturing and organizing user thoughts. The long-term direction is a structured thoughts, goals, and events system with possible CLI, HTTP, TUI, Markdown, and AI-assisted interfaces.

The core philosophy is:

> Build a boring, explicit Go + SQLite core where structured data is the source of truth; treat Markdown as a deliberate human-facing projection/editing layer; keep interfaces clean enough that CLI, HTTP, TUI, Markdown, and future agents can all share the same domain logic; move incrementally with TDD while preserving long-term architectural seams.

## Coding Principles

* Prefer small, explicit, idiomatic Go.
* Keep code boring and readable.
* Avoid clever abstractions, premature generalization, and framework-style magic.
* Use the standard library unless a dependency clearly earns its place.
* Keep functions narrow and easy to test.
* Pass `context.Context` through database and request-driven operations.
* Return errors clearly; do not hide failures.
* Do not introduce global state unless there is a strong reason. Justify that reason before adding.

## Architecture Principles

* The database is the canonical source of truth.
* Markdown should be treated as a projection, import/export format, or editing surface, not the hidden domain model.
* Keep domain/application behavior separate from delivery mechanisms.
* HTTP handlers, TUI commands, CLI commands, Markdown sync, and future AI surfaces should call shared application/domain logic instead of duplicating behavior.
* Do not put business logic directly into HTTP handlers when it belongs in an application/service layer.
* Prefer small interfaces that describe behavior the app actually needs.
* Avoid giant interfaces that mechanically mirror database tables.

## Feature Development Scope

For small feature pull requests:

* Implement one focused behavior at a time.
* Prefer incremental changes over large rewrites.
* Preserve existing package boundaries unless there is a clear reason to change them.
* Add or update tests with the feature.
* Keep migrations, store code, handlers, and tests consistent when schema-backed behavior changes.
* Do not introduce speculative infrastructure for future features unless the current feature needs it. Justify it before making changes.
* Leave clear seams for future CLI, HTTP, TUI, Markdown, and AI surfaces, but do not build all surfaces at once.

## Testing Principles

* Use TDD-style red, green, refactor when practical.
* Tests should support good design, not excuse poor design.
* Handler tests may use stubs/fakes.
* Store tests should verify real SQLite behavior where persistence matters.
* Prefer behavior-focused tests over tests that only mirror implementation details.
* Add regression tests for bugs before fixing them when practical.
* Integration tests are critical and should be run often after feature development.

## SQLite Principles

* Treat SQLite as a serious persistence layer, not a throwaway dev database.
* Use migrations for schema changes.
* Keep schema changes intentional and reviewable.
* Preserve foreign keys, constraints, indexes, and timestamp conventions already established in the project.
* Do not make broad schema redesigns inside a small feature PR unless explicitly requested.

## Markdown Principles

* Markdown is an intentional product direction.
* Markdown should remain mostly human-readable and useful.
* Do not make raw Markdown parsing the center of core app logic.
* Prefer this flow:

```text
DB/domain model -> Markdown projection
Markdown edits -> parsed commands/changes -> validated domain operations -> DB
```

* Avoid this flow:

```text
Markdown file -> fragile parser -> hidden app state
```

## Pull Request Expectations

A good PR should be:

* small,
* testable,
* readable,
* focused on one behavior,
* consistent with existing style,
* and easy to review.

When uncertain, choose the simplest implementation that preserves the architecture.

