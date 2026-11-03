# Goals interface

Open Goals from the Home entity strip. The native terminal and browser use the
same Bubble Tea component and application services.

## Browse and open

Use arrows, `j/k`, Page Up/Down, or the mouse wheel to browse Goals and Enter
for Details. `n` opens creation. Goals initially show Active with All priorities;
`f` cycles status and `p` cycles priority while the Goals list owns focus.
Priority filters remember their last selected Goal.

Tab moves between the Goals panel, status buttons, and Subject filter. Down from
status focuses Subject; Up returns to status. Up from the first Goal focuses
Priority, and Down returns to the Goal list. Right on Subject opens its searchable
picker. Left/Right page within it; a fresh Left on the first page closes the
picker without leaving Goals. Held Left stops at that boundary. Esc also closes
it. On the main Goals list, Left pages backward and a fresh Left on the
first page returns to Events. Held repeats stop at the first page; see
[native terminal limitations](platform-support.md#held-keys-during-goal-navigation).
Paging retains the highlighted row position. Active Goals sort before Inactive
Goals, with High → Normal → Low ordering within each group.

Cards show up to three live Subjects, target/cadence, status, priority, and any
configured date boundaries. Details shows every live Subject and record metadata.
No live Subjects means **Misc**, independently of Subject names. A failed read
shows an error and retry action, never a fabricated Misc assignment.

The list is loaded in one cancellable command: the existing Goal list call,
followed by live membership reads. There is no membership cache or bulk API.

## Create and edit

Up/Down moves between Name, Hours, Minutes, optional start/end dates,
Cadence, Priority, Status, and actions. Enter activates a control or saves from a text field;
Ctrl+S saves. Dates use YYYY-MM-DD. Create/Edit use the standard colored title block; the
focused field or action label is orange. Tab no longer moves between form fields. Cadence, Priority, and Status cycle using Left/Right or Enter.
Creation defaults to weekly and today’s local start date, which can be cleared.
Priority defaults to Normal and Status to Active; both can be changed before
creation. The end date stays empty. Both date labels show an example; form controls have
a blank line between them. An empty Subject selection is valid.

Hours and Minutes accept nonnegative whole numbers, including 90 minutes. A
positive total is required and overflow is rejected. Existing sub-minute target
precision is retained when duration inputs are unchanged; changing duration
uses whole minutes, as explained in the form.

Creation's Subjects action opens a multiselect. Arrows navigate, Space toggles,
`/` filters, Enter confirms, and Esc cancels. Selection survives filtering.
Confirming this picker only changes the creation draft.

From Details, `e` edits properties; `s` independently manages Subjects. Property
edits include Priority (High, Normal, Low) and Status (Active, Inactive).
Timezone, week start, and default cadence remain hidden and retain their persisted
values. Subject membership is edited independently.
Subject management preselects current live membership and saves the complete
selection. Historical links remain managed by PR #97's persistence contract.

Creation, property edits, and membership edits call GoalAssignments.Create,
Goals.Update, and GoalAssignments.ReplaceSubjects respectively. They are separate
operations; the TUI implements no relationship transactions.

Esc cancels before submission. While a write is pending, editing, duplicate Save,
and local cancellation are blocked. Application-session cancellation still ends
owned work. Errors preserve drafts. Conflicts offer explicit reload (discarding
unsaved changes) or cancellation; stale saves are never retried automatically.

## Review and limits

Focused model tests and a real migrated SQLite/root-TUI workflow cover creation
with multiple Subjects, property updates, Subject replacement, and return Home.
Browser acceptance uses the existing harness. Native terminal captures and browser
screenshots are transient artifacts outside the source tree.

This interface does not record progress or manage parents. Priority and Status
are changed through Create/Edit; the list’s `p` shortcut only filters. Very long Details and forms use Bubbles viewport scrolling;
filtering and pagination use Bubbles lists. No schema changes are needed.

## Shared Subject layout and creation

The Subjects browser and Goal picker share heading, always-visible search,
Subject-name column, two-line rows, and footer geometry. Compact windows retain
context and cancellation controls. Separator weight follows accessibility settings.
Browse rows retain thought counts; picker rows show selection without reading counts.

Create subject… is the picker's first action. Enter opens the existing Subject
creation form. Cancel returns to the unchanged picker; successful creation returns
with the new Subject checked and the Goal draft intact. This creates the Subject
only; Goal membership is persisted when the user confirms the picker or creates
the Goal. Ctrl+S confirms a selection independently of the highlighted action,
including an empty selection when no Subjects exist.

## Future Goal distribution curve

A future, separately scoped feature can explore a distribution curve around
Goals on the Goals page, building on their highlighted natural widths and the
[visual data guidance](app-style-guide.md#visual-data-and-content-shape).
First agree what the curve represents: the shape of rendered text or a defined
Goal metric. Keep its relationship to the Goal rows clear and preserve text,
focus, filtering, and narrow-window navigation. This is a design idea, not an
implemented Goal visualization or progress feature.

## Stress demo

`make tui/browser/demo/stress` now supplies 48 Goals and 32 live Subjects alongside
721 Events and 20,000 Thoughts. Goals cover every cadence/priority, active and
inactive states, optional dates, Unicode/long names, and zero-to-many memberships.
The first high-priority Goal includes all 32 live Subjects for Details/picker
scrolling. Historical-only and empty associations both exercise Misc presentation.
Mouse wheel and keyboard paging move through lists without changing checkbox
state; Space toggles a Subject and confirmation persists the selection.
