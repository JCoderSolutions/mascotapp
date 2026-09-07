---
type: convention
score: 4
topic_key: mascotapp/convention/tool-absence-must-fail-at-session-start
task: T-02-005
status: guardado
observation_id: obs-e03b5d9cf88abd24
rationale: "A tool that disappears mid-session is not the expensive part -- the expensive part is that its absence was reported to nobody. The `command not found` went to stderr inside a pipeline whose last command exited 0, so the session kept going and recorded work that had not happened. Any future session that runs a dependency inside a pipe reproduces this exactly, and the damage is not a failed step but a FALSE RECORD of a successful one."
---

# A missing tool must fail at session start, in the foreground — never inside a pipe

On 2026-09-02 `gentle-ai` vanished from the development host in the middle of a session.
The SDD `settle` that needed it ran inside a pipeline; its `command not found` went to
stderr, the pipeline's last command exited 0, and the session carried on believing the
attempt ledger had been updated. The attempt is still unsettled.

Two properties of a shell conspire here, and both are ordinary:

1. **A pipeline returns the status of its LAST command.** `some-tool | rg pattern` reports
   `rg`'s success, not `some-tool`'s absence. This project has now been bitten by this
   twice: once reporting a red test suite as green, once losing a tool entirely.
2. **`command not found` goes to stderr**, which a pipe does not carry, so the message is
   not even in the text being filtered.

The fix is not "be careful with pipes". It is a **preflight that runs before any work**:
`make doctor` (`scripts/doctor.go`), invoked with no pipe at all, which reports every
external dependency and what breaks without each one.

Three properties make it worth having rather than decorative:

- **It executes each tool instead of looking it up on `PATH`.** Presence is not
  runnability. On this host Smart App Control lets an unsigned binary sit on disk and
  refuses to run it, so a `LookPath` check would report a healthy toolchain while
  `go test` cannot start.
- **It fails only on REQUIRED tools.** A missing optional tool is reported in full — with
  the sentence saying exactly what stops working — and does not block. A preflight that
  blocks on everything gets skipped, and a skipped preflight is worth nothing.
- **Its manifest is bound to the Makefile.** If a recipe starts invoking a tool the
  manifest does not know about, doctor reports that as a finding against itself. A
  hand-written list that nothing checks drifts, always in the permissive direction.

## The correction the first run produced

`rg` was in the manifest and came back `FAIL: not on PATH` — while it was working
perfectly. `rg` is a **shell function injected by Claude Code**, proxying to the ripgrep
bundled inside its own binary; there is no `rg` on `PATH` and there never will be.

It was removed rather than special-cased. **A check that is permanently red on a healthy
machine teaches its reader to skim past warnings**, which is the exact habit the program
exists to break. Only check something whose absence means something.

Related: [[a-pipe-eats-the-exit-code]], [[makefile-tool-provenance]].
