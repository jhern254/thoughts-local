# Goals interface

Open Goals from the Home entity strip. The native terminal and browser use the
same Bubble Tea component and application services.

## Browse and open

Use arrows, Page Up/Down, or the mouse wheel to browse Goals and Enter for Details. `n` or Create Goal opens the
creation form. `f` cycles All, Active, and Inactive; `/` searches Goal names.
Esc clears a filter before returning to Home. Lists retain Goal priority order.

Cards show up to three live Subjects, target/cadence, status, priority, and any
configured date boundaries. Details shows every live Subject and record metadata.
No live Subjects means **Misc**, independently of Subject names. A failed read
shows an error and retry action, never a fabricated Misc assignment.

The list is loaded in one cancellable command: the existing Goal list call,
followed by live membership reads. There is no membership cache or bulk API.

## Create and edit

Tab/Shift+Tab moves between Name, Hours, Minutes, optional start/end dates,
Cadence, and actions. Enter activates a control or saves from a text field;
Ctrl+S saves. Dates use YYYY-MM-DD. Cadence cycles using arrows or Enter.
Creation defaults to weekly and today’s local start date, which can be cleared.
The end date stays empty. Both date labels show an example; form controls have
a blank line between them. An empty Subject selection is valid.

Hours and Minutes accept nonnegative whole numbers, including 90 minutes. A
positive total is required and overflow is rejected. Existing sub-minute target
precision is retained when duration inputs are unchanged; changing duration
uses whole minutes, as explained in the form.

Creation's Subjects action opens a multiselect. Arrows navigate, Space toggles,
`/` filters, Enter confirms, and Esc cancels. Selection survives filtering.
Confirming this picker only changes the creation draft.

From Details, `e` edits properties; `s` independently manages Subjects. Property
edits retain timezone, week start, default cadence, priority, and activation.
These internal settings and later priority/activation controls are not exposed.
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

This interface does not record progress, manage parents, toggle activation, or
change priority. Very long Details and forms use Bubbles viewport scrolling;
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

## Stress demo

`make tui/browser/demo/stress` now supplies 48 Goals and 32 live Subjects alongside
721 Events and 20,000 Thoughts. Goals cover every cadence/priority, active and
inactive states, optional dates, Unicode/long names, and zero-to-many memberships.
The first high-priority Goal includes all 32 live Subjects for Details/picker
scrolling. Historical-only and empty associations both exercise Misc presentation.
Mouse wheel and keyboard paging move through lists without changing checkbox
state; Space toggles a Subject and confirmation persists the selection.
