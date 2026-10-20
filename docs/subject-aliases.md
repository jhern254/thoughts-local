# Subject names and aliases

One canonical Subject owns each alias. Aliases resolve to the existing Subject
ID; they do not own or transfer Thoughts, Events, Goals, or progress.

Subject Details → **a: aliases** opens Names & Aliases. Enter adds or edits an
alias, **d** requests removal, and **Q/Esc** returns. Removal requires **y** and
changes only the alias. The canonical name uses the existing Subject editor.
Returning preserves the underlying Thought browser.

Names retain their exact case and spelling, including existing Subject whitespace
semantics. `coding`, `coDing`, and `Coding` remain distinct. An exact active name
cannot identify both a canonical Subject and an alias, even for the same Subject.
Existing pickers and filters are unchanged. `subject.Service.ResolveName` supplies
exact canonical/alias resolution; user-wide mappings are available through
`ListAliasesForUser`. Both require an active owner and canonical Subject.

Migration 000013 adds the alias table, same-owner composite foreign key, active
unique name index, per-Subject index, and validation triggers guarding both sides
of the canonical/alias namespace. SQLite write serialization and these database
guards protect competing writes; service-only checks would not suffice.
Known namespace conflicts retain the SQLite cause and identify
`data.ErrDuplicateRecord`; unrelated trigger failures remain errors.

Alias removal soft-deletes its row. Ordinary Subject deletion retires its aliases
within the existing deletion transaction, freeing their names while preserving
the current Thought/Event unlinking and historical relationship behavior.
Listing and resolution omit deleted aliases, Subjects, and owners. Apply schema
changes explicitly with the documented migration commands; startup does not
apply migrations.

Subject consolidation is a separate follow-up. No recommendation or AI workflow
is implemented here; future approved proposals must use the validated Subject
mutation operations.
