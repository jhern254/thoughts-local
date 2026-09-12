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

Operational logging accepts only approved typed metadata and uses fixed event messages. Never pass authored content, arbitrary objects, or raw error text into logs. Classify existing errors at the presentation boundary; services and stores return errors without logging them.

* The database is the canonical source of truth.
* Markdown should be treated as a projection, import/export format, or editing surface, not the hidden domain model.
* Keep domain/application behavior separate from delivery mechanisms.
* HTTP handlers, TUI commands, CLI commands, Markdown sync, and future AI surfaces should call shared domain/application logic instead of duplicating behavior.
* Do not put business logic directly into HTTP handlers when it belongs in an application/service layer.
* Prefer small interfaces that describe behavior the app actually needs.
* Avoid giant interfaces that mechanically mirror database tables.

### Cross-entity coordination

When a use case combines multiple entities, put that coordination in a small, dedicated application service with narrow, consumer-defined interfaces. Keep each entity service focused on its own operations rather than accumulating dependencies for combined views or workflows.

* For example, a timeline service coordinates events and thoughts, and may later include goal progress. Event CRUD should not require a thought reader merely to support the timeline.
* Inject only the collaborators the coordinating service actually needs. Keep dependencies one-way; do not make entity services depend back on the coordinator.
* A narrow interface limits coupling but does not justify placing a dependency in the wrong service. Review responsibility before adding a constructor dependency.
* Unrelated CRUD tests needing placeholder `nil` arguments or irrelevant fakes are a design warning. Reconsider the service boundary instead of adding optional dependencies or no-op collaborators to hide the problem.
* Keep SQL and persistence filtering in stores. Coordinating services express the use case; presentation code calls them rather than duplicating cross-entity rules.
* Test coordination separately with focused stubs, retain independent entity tests, and verify persistence behavior with real SQLite. Required collaborators should be supplied in coordination tests.
* This is not a rule to create a service for every foreign key or hypothetical feature. Introduce coordination for a concrete use case, without generic frameworks or speculative dependencies.
* Coordinating calls does not make writes atomic. When a workflow requires all-or-nothing changes, preserve an explicit transactional persistence boundary rather than relying on sequential service calls.

## Feature Development Scope

For small feature pull requests:

* Follow the order of red green refactor loop to iteratively work towards working feature
* Red is used locally to prove the test matters; committed states should normally be green.
* Add or update tests with the feature.
* Implement one focused behavior at a time.
* Prefer incremental changes over large rewrites.
* Preserve existing package boundaries unless there is a clear reason to change them.
* Keep migrations, store code, handlers, and tests consistent when schema-backed behavior changes.
* Do not introduce speculative infrastructure for future features unless the current feature needs it. Justify it before making changes.
* Leave clear seams for future CLI, HTTP, TUI, Markdown, and AI surfaces, but do not build all surfaces at once.

## Testing Principles

* Use TDD-style red, green, refactor.
* Tests should support good design, not excuse poor design.
* Handler tests may use stubs/fakes.
* Store tests should verify real SQLite behavior where persistence matters.
* Prefer behavior-focused tests over tests that only mirror implementation details.
* Add regression tests for bugs before fixing them when practical.
* Integration tests are critical and should be run often after feature development.

### Testing during development

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

### TUI acceptance and visual testing

Test the intended interaction and appearance, not just the current implementation.
Passing tests or snapshots alone do not establish visual correctness. Use this
workflow for TUI changes, scaling coverage to the behavior and risk involved:

1. **Define acceptance rules before implementation.** Describe what the user
   sees before and after the affected actions, including focus, selection,
   scroll position, and return navigation. For substantial layouts, agree on a
   small annotated example. State spatial relationships explicitly: a calendar
   card is anchored at its start, touching intervals share one boundary label, and
   expansion preserves the time column while moving later content down.
2. **Write focused, deterministic presentation tests.** Reuse existing fakes;
   fix terminal dimensions, clock, timezone, and fixture data. Assert observable
   relationships in rendered rows/columns as well as state, not merely that
   labels exist. Derive expectations from the acceptance rules, not the layout
   helper being tested. Use terminal cell widths rather than byte counts for
   Unicode geometry. Keep database semantics in SQLite tests; do not add a
   database setup for each visual case.
3. **Exercise transitions through root dispatch where routing matters.** Send
   actual keys and execute the returned data commands, including normal Bubbles
   batches. Verify the resulting view, active panel, selection, and scroll
   anchor. For async ownership regressions, obtain a real command result and
   deliberately deliver it after navigation or a newer request. Deliver clock
   ticks and controlled replies explicitly instead of sleeping; do not run
   recurring timers indefinitely or add a generic test message framework.
4. **Use a few reviewed snapshots as a supplement.** Keep representative normal
   and narrow layouts. Inspect every changed golden against the acceptance
   rules; never regenerate expectations solely to make tests pass. Preserve
   whitespace when testing geometry. ANSI-stripped text cannot verify color or
   focus highlighting; use focused style assertions and terminal review for
   those behaviors.
5. **Run the actual app before handing off visible changes.** Use synthetic data
   in a disposable migrated database, reusing the seeded demo when available.
   For substantial screens, inspect normal and narrow terminal sizes and resize
   during interaction. Walk through entry, panel switching, scrolling,
   expansion/detail and return, and any changed create/save/cancel flow. Include
   empty, populated, and overflow cases as relevant—not just one happy-path
   record. Choose feature-specific boundaries, such as adjacent/gapped events,
   midnight, or long Unicode input; avoid an exhaustive cross-product. Test
   loading/error states with controlled fakes. Check displayed help and focus
   styling; verify terminal restoration when changing startup/quit behavior.
6. **Review and report evidence.** Inspect terminal captures before and after
   the important actions; use a short terminal recording when timing, redraw,
   or flicker matters. A recording without review is not validation. Use only
   synthetic content, and keep transient captures outside the source tree.
   Report the scenarios, dimensions, checks actually run, and any unverified
   behavior or unavailable terminal tooling. Do not claim a visual review from
   string assertions alone. Turn manually discovered bugs into focused
   regressions before fixing them where practical, then repeat the affected
   walkthrough and run the required repository checks.

Prefer existing Go tests and terminal tools over a new UI-testing framework.
A small key or wording correction needs its affected interaction checked, not
an unrelated full-screen test matrix. New tooling or broad harness work should
be justified separately rather than bundled into a feature PR.

### Layered test structure

Use each test layer for a distinct purpose:

```text
service unit tests
    -> local stubs/spies
    -> isolated business behavior

handler/application tests
    -> reusable testutils fakes for normal behavior
    -> local stubs only for controlled failures

SQLite integration tests
    -> real migrated temporary SQLite DB
    -> independent behavior-focused subtests
```

### SQLite integration-test organization

Integration tests verify real component interactions and database behavior, but each observable behavior should remain independently testable.

* Use real SQLite for persistence integration tests and run the real migrations before exercising stores or services.
* Prefer a fresh temporary migrated database for each independent scenario. Do not share mutable database state between unrelated behaviors.
* Organize tests as `Test<Feature>Workflow_<Infrastructure>` with `t.Run("<observable behavior>")` subtests. For example:

```go
func TestSubjectWorkflow_SQLite(t *testing.T) {
    t.Run("creates and retrieves subject", ...)
    t.Run("enforces per-user uniqueness", ...)
    t.Run("deletes subject and unlinks linked thoughts", ...)
}
```

* Name subtests after observable behavior, not implementation mechanics.
* Do not build one giant sequential workflow whose later assertions depend on all earlier mutations.
* A scenario may contain several operations when they are required to prove one meaningful behavior; keep those related operations together.
* Test SQLite-specific behavior against real SQLite rather than reproducing it in fakes.
* Verify important migration constraints, foreign keys, ownership rules, uniqueness, persistence, and delete/update semantics through integration tests.
* Reopen the database when persistence across connections is the behavior under test.
* Keep integration tests readable and explicit. Avoid generic harnesses and excessive abstraction.

## SQLite Principles

Soft-deleting a subject retains its record but atomically clears `subject_id` on all linked thoughts, including soft-deleted thoughts. This changes assignment, not history: retain thought-tag associations, mindset periods, and event/goal-progress provenance.

* Treat SQLite as a serious persistence layer, not a throwaway dev database.
* Use migrations for schema changes.
* Keep schema changes intentional and reviewable.
* Preserve foreign keys, constraints, indexes, and timestamp conventions already established in the project.
* Do not make broad schema redesigns inside a small feature PR unless explicitly requested.

* Database schema changes use golang-migrate.

Create migrations in migrations/.
Apply them with:

```text
    make migrate/up
```

Do not implement custom migration runners or run migrations
implicitly during API startup.

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

## Branches and Worktrees

Each agent MUST work in its own Git branch and Git worktree.

Never have multiple agents modifying the same branch or working directory.

Workflow:

1. Update `main`.

   ```bash
   git switch main
   git pull --ff-only
   ```

2. Create a new worktree and feature branch.

```bash
git worktree add ../thoughts-<feature> -b feature/<feature>
```

3. Perform all work inside that worktree.

4. Commit only changes related to the assigned task.

5. Push the feature branch and open a Draft Pull Request.

6. Before requesting review:

```bash
git fetch origin
git rebase origin/main
make check
```

7. Resolve any conflicts on the feature branch.

8. CI must pass before merging.

9. After the branch is merged:

```bash
git worktree remove ../thoughts-<feature>
git branch -d feature/<feature>
```

Rules:

- One worktree per agent.
- One feature branch per worktree.
- Never commit directly to `main`.
- Never share a branch between multiple agents.
- Keep pull requests small and focused.
- Rebase onto the latest `main` before requesting review.

## Git Workflow

- Work on feature branches for new features.
- Open pull requests for review.
- Never merge into `master`; the project owner handles final merges.
- Keep commits small and focused when practical.
- Do not commit failing or red states.
- Prefer clear commit messages that describe the actual change. 

Follow this workflow:
```
Before committing, run `make quick`.

Before pushing or requesting review, run `make check`.

Do not claim that a task is complete unless the required checks pass.
Do not weaken, skip, or delete tests merely to make the branch pass.
CI is authoritative and must run `make ci`.
```


Example:
```
Feature: create goal

Local Red:
- Add a failing test for creating a goal.

Green:
- Add minimal schema/store/service/handler code needed to pass.

Commit:
- add: create goal()

Refactor:
- Implement changes, clean names, reduce duplication, improve package boundaries.

Commit:
- refactor: create goal()
```

### Commit Message Conventions

Use short, plain commit prefixes:

- `test:` for adding or changing tests.
- `add:` for simple new behavior, files, routes, structs, or green-path additions.
- `fix:` for bug fixes or failing behavior corrections.
- `refactor:` for restructuring existing implementation without changing behavior.
- `docs:` for documentation-only changes.
- `schema:` for SQLite migrations or schema changes.
- `chore:` for maintenance, tooling, formatting, dependency updates, or cleanup.

Examples:

```text
test: add subject creation handler test
add: implement subject creation endpoint
fix: return not found for missing subject
refactor: move subject validation into data package
schema: add goals table migration
docs: add project agent instructions
chore: format go files
```

### Scope discipline

PRs must remain narrowly focused on the stated goal.

Implement only what is required for the current PR.
Do not opportunistically refactor unrelated code.
Do not implement future entities, features, infrastructure, or abstractions unless the current task requires them.
Do not use an existing PR as an excuse to clean up neighboring code.
If a larger architectural improvement is useful but not required, ask if it should be separate focused PR.
Prefer several small, reviewable PRs over one broad PR.

A PR should be explainable in one clear sentence.

### Commit discipline

Each commit must be local to one coherent goal.

A commit should contain only changes necessary for its stated purpose.
Do not mix feature implementation, unrelated refactors, formatting churn, cleanup, and architectural changes in one commit.
Avoid touching files that do not need to change.
Do not rewrite working code merely to make it stylistically different.
Commit messages should describe the actual focused change.

Prefer commits that are easy to review independently.

### Minimal implementation

Prefer the smallest clean implementation that correctly solves the problem.

Minimize lines of code changed and added.
Avoid fluff code, speculative helpers, wrapper layers, generic frameworks, and abstractions that are not currently necessary.
Prefer deleting obsolete plumbing over adding abstractions around it.
Use straightforward, idiomatic Go.
Prefer explicit dependencies and obvious control flow.
Use narrow interfaces.
Avoid premature generalization.
Do not create generic infrastructure for a problem that currently has one or two concrete cases.
Follow established Go best practices rather than blindly copying legacy patterns in the repository.

Minimal does not mean compressed or clever. Code should remain readable and maintainable.

The preferred outcome is:

few concepts
few abstractions
few changed lines
clear responsibilities
idiomatic Go
Existing code is context, not authority

Legacy code may represent transitional or obsolete design decisions.

Do not automatically reproduce an existing pattern just because it already exists in the repository.

Before extending a pattern, consider whether it matches the current architecture and documented project direction and ask about changes.

Prefer the intended architecture over preserving accidental legacy structure.

Database schema is authoritative

SQL migrations are the source of truth for persistent schema constraints.

Go models and validation must mirror the migration contract rather than inventing parallel schema rules.

When implementing or modifying an entity, compare the Go representation against the migrations for:

field types
nullability
minimum and maximum values
text-length constraints
defaults
uniqueness
foreign keys
version constraints
timestamps
allowed values

Do not silently introduce a stricter or different application-level limit.

For example, if a migration defines:

CHECK (length(trim(subject_name)) BETWEEN 1 AND 255)

Go validation must not independently introduce a 120-byte limit.

Also preserve SQL semantics where practical. For SQLite text length constraints, remember that Go len(string) counts UTF-8 bytes, while SQLite length(TEXT) counts Unicode characters/code points. Use rune-aware validation where matching the migration requires it.

Application validation may provide earlier and clearer errors, but it should reflect the database contract.

If an intentionally stricter business rule is ever needed, it must be deliberate and documented rather than invented during implementation.

Tests drive feature implementation

Tests are a core part of implementing a feature, not an afterthought.

For new behavior:

identify the behavior being added or changed,
write or update the relevant test,
implement the minimum production code necessary to satisfy it,
refactor only when useful while preserving behavior.

Preserve a red-green-refactor mindset where practical.

A feature is not complete merely because the production code compiles.

Test organization

Tests in this repository should be behavior-focused and organized using descriptive grouped subtests.

The preferred structure is:

```
func Test<Type>_<Method>(t *testing.T) {
    t.Run("describes expected behavior", func(t *testing.T) {
        // arrange
        // act
        // assert
    })

    t.Run("describes another behavior", func(t *testing.T) {
        // arrange
        // act
        // assert
    })
}

Examples:

func TestSubjectService_Create(t *testing.T) {
    t.Run("creates normalized subject", func(t *testing.T) {
        // ...
    })

    t.Run("rejects invalid subject before persistence", func(t *testing.T) {
        // ...
    })

    t.Run("returns store error", func(t *testing.T) {
        // ...
    })
}

func TestFileSystemStore_GetSubject(t *testing.T) {
    t.Run("returns subject", func(t *testing.T) {
        // ...
    })

    t.Run("returns not found", func(t *testing.T) {
        // ...
    })
}
```

Prefer:

```
Test<Type>_<Method>
    t.Run("<behavior>")
```

rather than creating a separate top-level Test... function for every individual condition.

The subtest name should describe observable behavior rather than implementation details.

Good:

"creates normalized subject"
"returns 404 for missing subject"
"rejects invalid thought before persistence"
"returns duplicate record error"

Less useful:

"test one"
"invalid"
"error case"
"works"

### Integration tests

Integration tests must use the real migrated SQLite database and be organized as independent, behavior-focused subtests. Each unrelated scenario should start with a fresh temporary database so failures remain isolated. Prefer `Test<Feature>Workflow_<Infrastructure>` with descriptive `t.Run("<observable behavior>")` cases, and test database-specific behavior such as constraints, foreign keys, ownership, uniqueness, and persistence against SQLite rather than reproducing it in fakes.

Integration assertion failures should identify the field or operation and report both the actual result and the expected value or condition (`got …, want …`). Prefer `if got, want := …; got != want` for simple comparable values when it improves readability. For predicates such as nonzero timestamps, containment, or error identity, keep the appropriate comparison and describe the expected condition. Setup and cleanup errors may use `t.Fatal(err)` or `t.Error(err)`. This is an integration-test diagnostic guideline, not a mandatory assertion format for every unit test.

### Feature growth and logging

Reuse existing infrastructure; avoid duplicated policies, per-entity wrappers,
and redundant registries. Small explicit allowlists are appropriate for privacy.

Use the injected `internal/logging` adapter for new operational logs. Classify
errors with `failure.Classify` at presentation/lifecycle boundaries, log once
when requested, and preserve original errors. Services and stores return errors
without logging.

Emit only approved typed metadata—never authored content or raw error text.
Reuse existing categories; add events or classification rules only when the
implemented feature requires them.

### Feature growth and logging

Reuse existing infrastructure; avoid duplicated policies, per-entity wrappers,
and redundant registries. Small explicit allowlists are appropriate for privacy.

Use the injected `internal/logging` adapter for new operational logs. Classify
errors with `failure.Classify` at presentation/lifecycle boundaries, log once
when requested, and preserve original errors. Services and stores return errors
without logging.

Emit only approved typed metadata—never authored content or raw error text.
Reuse existing categories; add events or classification rules only when the
implemented feature requires them.

Each entity reuses the injected logger, and the Bubble Tea
“wrappers” are small asynchronous commands around the shared service—not another business-logic
layer.

For a new persisted entity, the usual pieces are:

Piece                 What you add
━━━━━━━━━━━━━━━━━━━━  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Schema and model      Migration, Go representation, relationships, ownership, constraints, and any
                     soft-delete behavior.
────────────────────  ──────────────────────────────────────────────────────────────────────────────
Store                 SQLite operations and a narrow interface containing the operations the
                     feature needs.
────────────────────  ──────────────────────────────────────────────────────────────────────────────
Service               Validation and domain operations, with rules matching the schema.
────────────────────  ──────────────────────────────────────────────────────────────────────────────
Application wiring    Construct the store/service using the existing database connection and
                     inject the service into the relevant interface.
────────────────────  ──────────────────────────────────────────────────────────────────────────────
Presentation          TUI screens and interactions, or CLI commands/HTTP handlers for whichever
                     surfaces you’re implementing.
────────────────────  ──────────────────────────────────────────────────────────────────────────────
Tests                 Service behavior, real migrated SQLite integration tests, and presentation
                     behavior.

For Bubble Tea specifically, you need more than views:

- State: selected item, form input, loading state, and errors.
- tea.Cmd functions that call the service.
- Result messages carrying returned data and the original error.
- Update handling for those messages and keyboard actions.
- Navigation, focus, resizing, and refreshing lists after mutations.

These commands call ordinary service methods. Add service methods when the feature needs new domain behavior; navigation and form focus belong in the TUI.

For errors and logging, reuse the existing infrastructure:

- Existing common sentinels where their meaning fits.
- Entity validation errors with validator-produced public feedback.
- Safe presentation messages and generic unexpected-error fallbacks.
- failure.Classify plus the injected logger. Extend operation metadata/classification only when a new implemented operation requires it.




When uncertain, choose the simplest implementation that preserves the architecture.
