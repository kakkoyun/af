# ADR Drafts — Phase II.5

**Status:** Post-review drafts — decisions locked by user directives D1–D7
(see `gap-analysis.md §0` and `§0.2`) and the critic / security / architect review
synthesized in `gap-analysis.md §8`. Ready for the lead to author final ADRs in
`docs/adr/`. Once each is committed under `docs/adr/NNN-<slug>.md` with `Status:
Accepted`, delete the corresponding section here. When the file is empty, delete
the file.

**Ratification checklist for each ADR below:**

1. Lead translates the draft into Nygard format (Context / Decision / Alternatives /
   Consequences) at `docs/adr/NNN-<slug>.md`.
2. Each new ADR that supersedes part of another ADR states the supersession
   explicitly in the header metadata.
3. `docs/adr/README.md` index is updated.
4. A one-sentence PROGRESS.md entry notes the ratification.
5. The entry here is deleted (not marked "done").

---

## ADR-022: cmux Multiplexer Provider

**Kind:** Design. **Size:** M. **Drives:** gap G7, directive D3. **Status:** Ready
to author.

### Context

`af` uses tmux as the sole `Multiplexer` trait implementation
(`src/mux/tmux.rs`). cmux is a macOS-native multiplexer with Unix-socket IPC,
native `cmux ssh` for remote workspaces, an RPC channel, and a tmux-compatible
primitive surface (`send`, `send-key`, `new-workspace`, `capture-pane`,
`resize-pane`, etc.). User directive D3: cmux and tmux are interchangeable
multiplexers selected via `[general] multiplexer = "tmux"|"cmux"`.

Critic [C 2.3] recommended deferring to 0.2.0 (four open cmux decisions, risk of
`CMUX_SOCKET_PASSWORD` leaking into the mux layer). User directive is
authoritative: **cmux ships in 0.1.0.** The critic's reasoning is captured in
§Consequences below so a future session sees why the open decisions matter.

### Decision

- **Option (1) from gap §8.3 — first-class `CmuxMux` impl.** No change to the
  `Multiplexer` trait (17 methods); no new capability flag. Each method maps to
  cmux's native primitive or its tmux-compat counterpart. Where cmux's `surface`
  concept is richer than tmux's `window`, `CmuxMux` projects down to the tmux
  model — the trait stays lossless for both backends.
- **Factory auto-select** in `src/mux/mod.rs`:
  1. `[general] multiplexer = "tmux"|"cmux"` if set.
  2. Else `$CMUX_WORKSPACE_ID` non-empty → cmux.
  3. Else `$TMUX` non-empty → tmux.
  4. Else whichever binary is on PATH (tmux preferred).
- **`CMUX_SOCKET_PASSWORD` handling**: read from the user's shell env at launch
  time. Never persisted by af. Out of scope for ADR-016 / ADR-025; the password
  is user-managed (cmux docs instruct the user to export it or use macOS
  Keychain integration that cmux itself owns).
- **cmux agent-opinionated subcommands** (`claude-teams`, `omc`, `omo`, `omx`,
  `codex install-hooks`) are **ignored** — `af` owns agent choice via
  `AgentProvider`. Listed in Non-Goals.

### Alternatives considered

- **(2) tmux-compat shim only.** Smaller surface but inherits cmux's quirks in
  `capture-pane`/`pipe-pane` timing. Rejected: first-class impl is cheaper once
  we pay the trait boilerplate once.
- **(3) defer to 0.2.0** per [C]. Rejected by user directive; cost of coupling
  (mux ↔ keyring) judged tolerable because `CMUX_SOCKET_PASSWORD` lives outside
  af's secret store.

### Consequences

- Users on macOS can opt into cmux's richer window/pane model without losing
  feature parity with tmux users.
- If cmux adds new primitives post-ship, they land inside `CmuxMux` without
  touching the trait.
- **Risk carried from critic:** coupling temptation. If a future contributor
  adds cmux-specific methods to the trait, tmux users regress. Mitigation: the
  `src/mux/mod.rs` trait is on the shared-files list; changes go through the
  lead, who re-checks cmux-can-implement AND tmux-can-implement.

---

## ADR-023: Sandbox Agent-Layer Conflict Resolution

**Kind:** Design. **Size:** S (~2 paragraphs — ratify shipped behavior).
**Drives:** gap G3. **Status:** Ready to author.

### Context

`src/provider/slicer.rs` mixes two slicer abstractions: `slicer vm {add,delete}`
for lifecycle + `slicer {claude,codex,amp,copilot}` for agent launch, falling
back to `slicer workspace` for unknown agents. `src/provider/docker.rs` calls
`sbx create` in `SandboxProvider::create()` and `sbx run` via
`agent_sandbox_cmd()` — technically a double-create because `sbx run` creates on
first use.

### Decision

- **Slicer:** Option (A) — **keep the current split.** Lifecycle is af-owned
  (so af can list/teardown without invoking agent-opinionated code);
  agent-subcommands leverage slicer's built-in agent setup. Shipped and tested;
  changing it without user-visible benefit would be churn.
- **sbx:** Option (X) — **drop `sbx create` from `SandboxProvider::create()`**,
  let `sbx run` handle creation on first use. Matches sbx's own docs. Closes gap
  G6 and removes the double-create.

### Consequences

- Lane S1 from the pre-review plan becomes a no-op (no code change for slicer).
- The sbx change is folded into **Lane L-FIX** alongside the `docker.rs:56`
  workdir/agent bugs. All three sbx fixes ship in one commit sequence before
  Phase II.5 opens.
- This ADR is ~20 lines and commits as a plain `docs(adr):` change.

---

## ADR-024: Remote Sandbox via Daemon URL

**Kind:** Design. **Size:** S. **Drives:** gap G1. **Supersedes:** ADR-014
§"Composition model" L37–41 for slicer. **Status:** Ready to author.

### Context

Slicer exposes `--url <URL>` / `SLICER_URL` + `--token <T>` / `SLICER_TOKEN`
daemon mode (verified `gap-analysis.md §1`). The user's shell already wraps
this: `slicer --remote=<host> <cmd>` → `command slicer <cmd> --url <resolved>
--token-file <path>`.

ADR-014's four-step composition pipeline (SSH → provision → sandbox → launch)
is correct for sbx+exedev but **wrong for slicer**: daemon mode has no SSH
install step, no provisioning pipeline. Per architect [A] §1: "provisioning is
the bridge" → "connection is the bridge" for daemon-based providers.

### Decision

- Add `remote_daemon: Option<RemoteDaemon>` field to `SandboxConfig`
  (`src/provider/mod.rs`):
  ```rust
  pub struct RemoteDaemon {
      pub url: String,
      pub token_source: TokenSource,  // File(PathBuf) | Env(String) | Keyring
  }
  ```
- Slicer provider reads `remote_daemon` and appends `--url` / `--token-file` (or
  `--token` when `Env`) to every invocation.
- sbx provider **ignores** `remote_daemon` for 0.1.0 (sbx has no daemon mode).
  Future sbx daemon support reuses the same field without trait change.
- `TokenSource::Keyring` integrates with ADR-025's scope (host-only keyring);
  daemon tokens are per-host, stored as `af/slicer/<host-alias>`.
- Feature-gated behind `slicer-remote` (already scaffolded in Lane D).

### Alternatives considered

- **Wrap exedev's SSH path** for slicer too. Rejected: duplicates provisioning
  that the slicer daemon already handles natively.
- **Treat slicer-local and slicer-remote as separate providers.** Rejected: the
  agent/workflow is identical; the only delta is URL presence.

### Consequences

- Lane L-SBX-DAEMON shrinks to "plumb `remote_daemon` through one struct + one
  test."
- `SandboxProvider` trait is unchanged.
- Future providers with daemon modes (hypothetical: remote `sbx`, remote
  `modal`) reuse the same field.

---

## ADR-025: Secret Boundaries (narrows ADR-016)

**Kind:** Design. **Size:** M. **Drives:** G2, D1, critical findings N1/N3 +
high findings H-a/H-b/H-c. **Extends:** ADR-016. **Status:** Ready to author
(security-critical; draft reviewed by security-reviewer agent).

### Context

ADR-016 defined keyring storage + env-var injection as the secret mechanism.
Subsequent research revealed:

1. **Env-var injection is wrong for sandboxed agents.** `sbx secret` uses a
   proxy that never exposes the secret to the agent (`sbx secret --help`:
   "The secret is never exposed directly to the agent."). Slicer has its own
   native `slicer secret` store. Workspaces has `workspaces secrets`. Injecting
   into a sandboxed session env is either redundant or outright wrong.
2. **SSH `SetEnv`/`SendEnv` is a data-leak vector** (finding N1). On multi-tenant
   exe.dev, API keys land in `/proc/<sshd-child>/environ` readable by any
   co-tenant. `sshd` debug configs can log `SetEnv` names.
3. **Plain `HashMap<String,String>` has no scrubbing guarantee** (H-a). One
   `tracing::debug!("env: {:?}", env)` or panic backtrace leaks the key.
4. **Linux Secret Service default collection is enumerable** (H-b). Any process
   in the unlocked user session can `SearchItems` and read all `af/*` entries.
5. **No rotation protocol** (H-c). `af auth clear` does not signal running
   agents; the clear is a false reassurance.

User directive D1 forbids af-level sync across provider-native stores.

### Decision

**Boundary rule (the decision, one sentence):** af's keyring stores secrets for
**host agents and exedev SSH sessions only**; every other path (sbx, slicer,
workspaces) defers to the provider-native secret store, with af merely pointing
the user at the correct CLI command.

**Concrete rules:**

1. **Keyring scope** (amends ADR-016 §Decision §"af auth subcommand design"):
   - Service name stays `af`. Account is `<provider>` (not `af/<provider>` —
     drops the redundant prefix per [C] 2.2).
   - Linux: use a **dedicated, non-default collection** via D-Bus
     `CreateCollection`. Auto-lock on idle. Opaque labels
     (`af-<uuid>` ⇒ lookup table in the collection attributes, not the label).
   - macOS: use the user's default Keychain (hardware-backed on Apple Silicon;
     `security` CLI exposes entries but user-root is the threat-model boundary).

2. **Delivery transport** (supersedes ADR-016 §Consequences L91–93):
   - **Host:** inject via `auth::inject(env, provider)` — env-var. Wrap values
     in `secrecy::SecretString`; `Debug` prints `[REDACTED]`.
   - **exedev (SSH):** **forbid `SetEnv`/`SendEnv`.** Write to
     `/run/user/$UID/af-<session>/.env` mode 0600 on the remote, have the agent
     read-once-then-unlink. Fallback: pipe on stdin to `af agent launch
     --read-env-from-stdin`. Both mechanisms leave no `/proc/*/environ` trace
     visible to sibling processes.
   - **sbx, slicer, workspaces sandboxes:** **do not inject.** `af doctor`
     checks the native store for the expected key; on miss, prints the exact
     CLI the user should run (`sbx secret set …`, `slicer secret create …`,
     `workspaces secrets set …`). af never touches those stores.

3. **Rotation + revocation protocol:**
   - `af auth clear --provider <name>` lists live sessions using that provider
     from the ledger, warns, and offers `--kill-sessions` to terminate them.
   - `af auth reroll --provider <name>` same behavior: `--kill-sessions` kills
     stale launches; otherwise the change applies to **new launches only**.
   - Documented explicitly: *"keyring changes affect new launches; running
     agents hold the key in memory until terminated."*

4. **Redaction enforcement:**
   - All secret values are `secrecy::SecretString` from retrieval to `execve`.
   - Grep gate in CI: deny `{:?}` formatting on types named `*Env*` or
     containing `Secret` as a field.
   - Panic hook strips env from backtraces.

5. **Threat model (new ADR §Threats — fills gap M3):**
   - In-scope: casual shell history exposure, accidental commit of `.env` files,
     basic malware in user session, co-tenant on multi-tenant remote.
   - Out-of-scope: root-level compromise, kernel-level attacks, physical
     access, adversarial sshd at provider level (that's the provider's
     threat model).

### Alternatives considered

- **af-level sync across provider stores** (the old ADR-025 draft). Rejected
  per D1 + security findings: expands blast radius, creates rotation hazard,
  audit trail diverges across N stores, `workspaces secrets set KEY VAL` as
  argv leaks via `/proc/<pid>/cmdline`.
- **Encrypted-file fallback when Secret Service is down.** Rejected (ADR-016
  already): would require its own key-management, out of scope.
- **User-provided encryption keys (à la `age` / `sops`)**. Rejected: increases
  setup friction against marginal security benefit; macOS Keychain + dedicated
  Linux collection is the threat-model-appropriate baseline.

### Consequences

- `af auth setup/reroll/status/clear` scope is **host + exedev only**.
  Documented in help text and the book.
- Sandboxed-agent workflow requires one-time secret setup in the provider-native
  store. `af doctor` guides the user. Not an af regression — it matches how
  each sandbox tool is designed to work.
- Secret values never appear in `{:?}` output, panic backtraces, or SSH
  `SetEnv` headers.
- `af auth clear` is honest about what it does and does not terminate.
- **Implementation cost:** `secrecy` + `zeroize` added as deps (behind `keyring`
  feature). Tmpfs delivery adds ~30 LOC to exedev bootstrap. Dedicated Linux
  collection adds ~40 LOC to `auth::init` (one-time `CreateCollection` if
  absent).

---

## ADR-027: Remote = SSH Target (narrows ADR-004 + ADR-017)

**Kind:** Design. **Size:** S. **Drives:** G11, D2, H-e. **Supersedes:** ADR-004
§30–44 (`RemoteProvider` trait surface) and ADR-017 §"probe" prose (provider
identity conflation). **Status:** Ready to author.

### Context

Per directive D2, "remote = SSH-able host" — exe.dev is not special. Both
exedev VMs and DD Workspaces VMs resolve to entries in `~/.ssh/config` after
their respective `create` call. The distinction between them is **lifecycle**
(create/list/teardown/suspend), not **connection**.

Architect [A] §1 observation: `RemoteProvider` currently mixes lifecycle with an
implied "produces an SSH target" contract. `setup(ssh_host, repo, branch,
git_root)` on the trait only applies to exedev — workspaces' own CLI owns
provisioning.

Security finding C2 (N2): `StrictHostKeyChecking=no` on ADR-017's probe enables
MITM credential capture when combined with the session's `accept-new`.

### Decision

**Narrow `RemoteProvider` trait:**

```rust
pub trait RemoteProvider: Send + Sync {
    fn name(&self) -> &str;
    fn create(&self, req: &CreateRequest) -> Result<SshTarget>;
    fn list(&self) -> Result<Vec<RemoteSession>>;
    fn teardown(&self, name: &str) -> Result<()>;
    fn detect(&self) -> Result<()>;
    fn ssh_target(&self, name: &str) -> Result<SshTarget>;
    fn is_alive(&self, name: &str) -> Result<Liveness>;
}

pub enum Liveness { Alive, Suspended, Unreachable, Unknown }
pub struct SshTarget { pub host: String /* alias in ~/.ssh/config */ }
```

- `setup(…)` is **removed from the trait** and moves to
  `ExedevProvider::bootstrap(…)` as a concrete method. Workspaces does not need
  it; the workspaces CLI owns bootstrap.
- `is_alive` returns the four-state `Liveness` enum. Workspaces' `Suspended`
  (VM exists but not SSH-reachable) is not an orphan. `af list` uses the
  distinction for its orphan column.
- **Universal probe** (in a new free function `src/provider/ssh.rs::is_alive(target,
  timeout)`): uses `StrictHostKeyChecking=accept-new` on **both** probe and
  session — per N2, `no` is never safe on paths that precede key transit.

**Per-provider liveness:**
- exedev: `SshTarget` → free-function SSH probe. 4-second connect timeout;
  returns `Alive` or `Unreachable`.
- workspaces: `workspaces list | grep <name>` first. If present + status
  suspended → `Suspended`. Else fall through to SSH probe.

**Orphan detection rule (Lane L-REMOTE):**
- `Alive` / `Suspended` → not orphan.
- `Unreachable` → orphan (the user can `af done --force`).
- `Unknown` → display as-is, do not auto-clean.

### Alternatives considered

- **Keep `setup()` on the trait; stub it for workspaces.** Rejected: stubbing a
  trait method is the "lies about what it does" smell the architect flagged.
- **New `VmLifecycle` trait separate from a `SshReachable` trait.** Rejected:
  real cost (two trait bounds everywhere a remote is used) with no benefit at
  2 providers.
- **`is_alive -> bool` with workspaces treating `Suspended` as `true`.**
  Rejected: loses the UX distinction in `af list`. The four-state enum is
  cheap.

### Consequences

- `RemoteProvider` trait is narrower and more honest.
- `accept-new` on probe closes C2 / N2.
- Lane L-REMOTE owns the probe + orphan + liveness changes in one lane (folds
  former A1 + A2 + B3 + B4).
- ADR-004 §30–44 and ADR-017 §"probe" L33 + L80–83 are superseded by this ADR.

---

## ADR-028: Agent-Level OS Sandbox (new, per D6)

**Kind:** Design. **Size:** S. **Drives:** G15, D6, D7. **Status:** Ready to
author.

### Context

Directive D6: agent-local sandbox modes are orthogonal to af's VM/container
isolation layer. Verified on this machine:

- Codex: `codex sandbox {macos|linux|windows}` (Seatbelt / bubblewrap or
  Landlock / restricted token) and `-s/--sandbox <MODE>` policy flag.
- Claude: `--dangerously-skip-permissions` help text states *"Recommended only
  for sandboxes with no internet access"* — implies Claude defers to the
  caller's OS sandbox, does not provide one.
- Amp, Gemini, Copilot, Pi: no CLI sandbox flag discovered.

End goal D7's "isolation for overnight" pillar needs this: `af create --yolo`
on a local bare host today gives the agent unfettered write access. Even
without af's VM sandbox, the agent's own OS sandbox provides meaningful
defense.

### Decision

Add a new CLI flag:

```
af create --agent-sandbox <none|os>
```

- **Default:** `os` when the agent supports it, `none` otherwise.
- **Per-agent mapping:**
  - codex → append `-s workspace-write` (or the codex default-safe policy — the
    ADR reserves the exact mapping to the per-agent module; subject to
    testing).
  - claude → **no-op** (claude defers sandboxing to the caller; af does not
    need to pass a flag). Documented in `book/src/agents/claude.md`.
  - amp, gemini, copilot, pi → no-op; `--agent-sandbox=os` silently degrades to
    `none` with an info-level tracing log.
- **Orthogonal to VM-sandbox layer.** Composes as:
  ```
  af create --sandbox --agent-sandbox=os --yolo   # belt + suspenders
  af create --agent-sandbox=os --yolo             # local, protected
  af create --yolo                                # warn (G16 guard)
  ```

### Alternatives considered

- **Implicit "always on when available."** Rejected: no opt-out path for power
  users whose agent sandbox conflicts with a legitimate tool (e.g., codex's
  Seatbelt blocks some project-local binaries). `--agent-sandbox=none` is the
  documented escape.
- **Map to the agent's full native flag surface** (codex's `read-only` vs
  `workspace-write` vs `danger-full-access`). Rejected for 0.1.0: the tri-state
  complexity buys little; `os` vs `none` covers the common case. 0.2.0 can add
  a `--agent-sandbox-policy` if users ask.

### Consequences

- Lane L-AGENT-SANDBOX is small (<50 LOC + per-agent tests).
- The G16 guard ("warn when yolo has no sandbox") becomes actionable: "run with
  `--agent-sandbox=os` or `--sandbox`."
- Users on agents without OS sandbox support lose no functionality — they just
  get `none` silently. `af doctor` can surface "your agent does not support OS
  sandbox" as an info message.
- Overnight-yolo has a documented safe-path configuration in the book.

---

## Addenda (not full ADRs)

### A-b: ADR-018 addendum — drop `CommandRunner` trait

**Status:** Ready to author as a small ADR (ADR-029) or as an inline
"Supersession" header on ADR-018 if the constitution permits.

Per critic [C 2.1]: the `CommandRunner` trait threads `Box<dyn CommandRunner>`
through every provider constructor (~24 call sites). The ADR's Context is
actually about **CI fragility from external tool availability** — which is
solved by feature gates alone. The trait solves a different problem
(unit-test determinism on shell-output branches) at real cost.

**Decision:** adopt **feature gates + `assert_cmd`** only. Drop the trait.
Providers call `process::Command` directly. Integration tests run under
feature gate when the external binary is available; unit tests stub at the
public provider surface, not at the process boundary.

If a specific provider later needs branch coverage on shell failure paths,
that provider introduces a local `CommandRunner`-style trait — not a
workspace-wide trait.

**Saves:** ~200 LOC + one coordination axis from every Phase III lane.

### A-c: `book/src/concepts/approval-modes.md`

**Owner:** Lane L-BOOK. **Drives:** G14, D5.

Lift ADR-012's per-agent mapping table verbatim into a user-facing book page.
Add: "Claude's native surface has six modes; af's tri-state maps to three.
`plan` mode is a 0.2.0 candidate." No ADR; this is pure documentation.

### A-d: Overnight-yolo guard in `src/cmd/create.rs`

**Owner:** Lead in Phase IV (touches `cli.rs` + `create.rs`). **Drives:** G16, D7.

Two-line policy guard: when `ApprovalMode::Yolo` is set AND there is no VM
sandbox AND there is no agent-sandbox, print a warning:

```
warning: --yolo on a local host without a sandbox leaves the agent with full
filesystem + network access. This is risky for overnight runs.

Recommended:
  af create --yolo --agent-sandbox=os     # OS-level isolation
  af create --yolo --sandbox              # VM-level isolation
  af create --yolo --sandbox --agent-sandbox=os  # both

To proceed anyway, pass --i-know-its-risky.
```

Unit-tested at the CLI layer; no ADR.

---

## Ratification plan

1. Lead authors ADR-022, 023, 024, 025, 027, 028 + ADR-029 (A-b addendum) in
   parallel on a single branch `phase-2.5-adr-revision`. ~2 hours.
2. `docs/adr/README.md` index is updated in one commit.
3. One PROGRESS.md entry covers the round.
4. This file is deleted when all six new ADRs land.
5. Lane L-FIX (docker bugs + sbx double-create) can land **before or during**
   step 1 — it is independent of any ADR.
6. Lane L-BOOK picks up A-c and A-d during Phase III.
