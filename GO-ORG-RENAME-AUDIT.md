# Audit: Go Package Rename Plan — correctness & care review

> **What this is:** a full audit of the Go workstream of the `mcpcat` → `agentcathq` rebrand,
> cross-checking three things against each other: (1) the verified research
> (`GO-ORG-RENAME-RESEARCH.md`), (2) the written plan (`REBRAND.md` — D1, D2, D12, D16, §2B,
> §2E, §3.5, §3.6, §4), and (3) the **actual state of the code and git repos** on disk.
>
> **Method:** three parallel Fable (`claude-fable-5`) agents — a filesystem/git ground-truth
> inventory, a plan-vs-research reconciliation, and an independent **empirical** re-test of the
> one finding that contradicts the current plan.
>
> **Bottom line:** the plan's *strategy* is correct and well-thought-out, but it carries **one
> refuted mechanism repeated in ~10 places**, is **missing 3 real safety rules**, and its
> **per-repo specifics (counts, line numbers, literals) are substantially wrong** — including
> two facts that change the risk picture: `mcpcat-go-api` has **no LICENSE file**, and
> `agentcat-go-api` is **already scaffolded** locally.

---

## A. The crux, now settled empirically (was: D16's "narrow accepted exception")

D16 claims a cold `GOPROXY=direct` fetch of the **old** import path **fails** post-rename
"because the redirect does NOT preserve the module path," and books this as an accepted
permanent breakage. **This is false.** A Fable agent reproduced the exact scenario on live
`go1.24.4` / `git 2.39.5`:

- `github.com/docker/docker` **301-redirects** to `github.com/moby/moby`, and its `go.mod` still
  declares `module github.com/docker/docker` — i.e. *precisely* "org/repo moved, old go.mod
  left un-edited," our exact situation.
- `git ls-remote https://github.com/docker/docker` → **exit 0**, same HEAD as moby/moby (git
  follows the 301 transparently).
- `GOPROXY=direct GOSUMDB=off go get github.com/docker/docker@v24.0.7+incompatible` → **exit 0**,
  `go: added github.com/docker/docker v24.0.7+incompatible`. Pure git, no proxy. **Direct-mode
  fetch of the old path SUCCEEDS.**
- **The real failure mode, reproduced for contrast:** `github.com/coreos/bbolt` redirects but its
  `go.mod` declares `go.etcd.io/bbolt`; `go get github.com/coreos/bbolt@v1.3.5` → **exit 1**:
  ```
  parsing go.mod:
        module declares its path as: go.etcd.io/bbolt
                but was required as: github.com/coreos/bbolt
  ```
  The fetch *succeeded through the redirect*; the check that fails is the **go.mod path match**.

Two nuances the test surfaced:
- `go mod download mod@ver` does **not** enforce the path check; only build-graph ops
  (`go get`/`build`/`mod tidy`) do. So "download works" isn't proof — `go get` is, and it passed.
- Pre-`go.mod` versions escape the check (synthetic go.mod). Irrelevant here (we have real go.mod).

**Conclusion:** with the old `go.mod` left un-edited (already the plan), old pins resolve under
**both** default-proxy **and** `GOPROXY=direct`. **D1 has zero Go exceptions.** The plan is
*safer* than it currently claims.

---

## B. CONTRADICTED — must-fix inaccuracies in the plan

| ID | Where | Problem | Fix |
|----|-------|---------|-----|
| **C1** | D16 (L109) + repeated at D1 (L89), §2E (L178), §3.5 (L353), §3.6 (L362), §4 (L404, L461), §4.5 (L509), §4.6 (L512), header (L6) | The refuted "cold `GOPROXY=direct` old-path fetch fails / narrow accepted exception / path-vs-redirect check" framing (§A). "path-vs-redirect check" isn't even a real mechanism. | Replace with: *old pins resolve via BOTH proxy cache AND `GOPROXY=direct` (git follows the 301; un-edited go.mod path matches — empirically verified go1.24.4). Preconditions: old go.mod un-edited, old repos never deleted/recreated, no new tags on old repos.* Delete "narrow D1 exception" from D1/§4.5/§4.6. |
| **C2** | Changelog L547–548 | The 2026-06-23 adversarial reviews "corrected" *originally-accurate* text into the C1 error. | Don't rewrite history — append a dated entry documenting the reversal (research refuted it ×5 + empirical test). Re-check nothing else those two rounds touched under the wrong premise (Go-only; verified). |
| **C3** | §2B (L134) | Says un-retracted "because **retracting would block existing pins from resolving**" — false. `retract` doesn't delete or break builds (go.dev/ref/mod). | Keep the no-retract *decision*; fix the *reason*: retract wouldn't break builds, it mis-signals "defective version" (ours are fine). Bonus: retracting requires a new tag on the old repo, which M1 forbids. |

---

## C. MISSING / UNDER-ADDRESSED RISKS — add these rules

| ID | Risk | Evidence | Add to plan |
|----|------|----------|-------------|
| **M1** 🔴 | The plan says "un-edited & un-retracted" but never **"never push new tags to the old repos."** A new tag whose go.mod declares the agentcat path makes old-path consumers' `go list -u -m all` abort with `module declares its path as X but was required as Y`. | golang/go#41350, #42433; cmd/go source; §A empirical | D16 rules, §2E, §3.5/§3.6: *"NEVER push new tags (root or `mcpgo/`/`officialsdk/` prefixes) to the old repos after the rename."* |
| **M2** 🔴 | Ambiguity: §3.5 (L347) says "**new repo**" but changelog (L566) says "**Go repo renames**." For Go these differ critically: renaming the old repo to `agentcat-go-sdk` and tagging v1.0.0 there means the old path's redirect now reaches new, mismatched tags → the M1 failure. | Research §7; §A | Decide explicitly: **keep old repos at their current names** (they only move to the `agentcathq` org); create **separate NEW repos** for `agentcat-go-sdk`/`agentcat-go-api` v1.0.0. Do NOT rename the old repos to `agentcat-*`. |
| **M3** 🔴 | "Keep a detectable LICENSE in the old repos" is stated nowhere. Proxy permanence is **conditional on a detectable license**. **And `mcpcat-go-api` has NO LICENSE file at all** (see D) — so its already-published versions are only *temporarily* proxy-cached. | proxy.golang.org policy; codebase (§D) | §2E + §3.5/§3.6: keep MIT LICENSE present & detectable in old repos forever; **add a LICENSE to `mcpcat-go-api` before/at rename**; during the §2I copyright sweep don't break license detection. |
| **M4** | "Never **delete** the old repos" is only implicit; deletion breaks `@v/list`/upgrade queries even where cached versions still serve. | golang/go#37079 | Extend §2E: *never delete the old Go repos ever.* |
| **M5** | Submodule tag mechanics unstated: the new repo needs **three tags** — `v1.0.0`, `mcpgo/v1.0.0`, `officialsdk/v1.0.0`. | Research §4 (weakest-evidenced); codebase confirms prefixed tags exist today | §3.5: tag root + both submodule prefixes at the new path; rewrite examples' `replace` dirs (dev-only). |
| **M6** | "Re-badge pkg.go.dev" captures the *why* but not the *do a proxy fetch + page visit to trigger fresh indexing* for root + both submodules. | Research §5/§7 | §2E: after tagging, `go mod download` the 3 new paths, hit their pkg.go.dev pages, re-run GoReportCard, then re-badge. |
| **M7** | Vanity import path (`go.agentcat.com/...`) never considered — it would make the *next* rename cost zero. This is a **now-or-never** call: adopting it after tagging `github.com/agentcathq/...@v1.0.0` is itself another path change. | Research §6 (medium, secondary) | Add a decision line to D16/§3.5 (adopt, or explicitly decline) **before** tagging v1.0.0. |

---

## D. GROUND-TRUTH discrepancies (plan's per-repo facts vs. the actual code)

These don't change strategy but will misdirect execution if used as a checklist. Verified on disk:

1. **`agentcat-go-api` already exists locally** — module `github.com/agentcathq/agentcat-go-api`,
   package `agentcatapi`, already emits `agentcat_version`/`GetAgentcatVersion`, **no remote, no
   tags, no LICENSE**, one "pre-generation skeleton" commit. The plan (D12/§3.6) treats this as
   not-yet-created. Reconcile: is this the intended new-repo seed? It needs a LICENSE and, per
   M2, publication only post-rename.
2. **`mcpcat-go-api` has NO LICENSE file** (go-sdk has MIT). Breaks the "MIT ⇒ permanent proxy
   cache" assumption *for the API client specifically* — and the SDK transitively requires
   `mcpcat-go-api@v0.1.7`. → M3.
3. **Reference counts are far off.** go-sdk `github.com/mcpcat`: plan "77" → **157 occurrences /
   79 files** (140 lines excl. go.sum + this doc). go-api: plan "83" → only **3** `github.com/mcpcat`
   refs (98 case-insensitive *brand* mentions — the plan likely conflated the two).
4. **Event literal:** `mcpcat:custom` **does not exist** in this SDK; only `"mcpcat:identify"`
   (`internal/event/helpers.go:109`). Plan's D3/§3.5 example is wrong for Go.
5. **`MCPCAT_SOURCE` / telemetry `source` field do not exist** in the Go SDK at all (the §2C/§3.5
   "source → agentcat" task is a no-op for Go). Closest analog: `SdkLanguage="Go"`.
6. **Line numbers:** `MCPCAT_API_URL` read is `mcpcat.go:111` (plan says :108); `config.go:11`
   default `https://api.mcpcat.io` ✓; `MCPCAT_PROJECT_ID` examples-only ✓; `mcpcat_version` at
   `internal/core/types.go:104` + go-api `model_publish_event_request.go:67` ✓.
7. **Tags richer than stated:** go-sdk has `v0.1.0–v0.3.0` **plus** `v0.1.1`, `v0.1.2`,
   `mcpgo/v0.3.0`, `officialsdk/v0.3.0`; go-api has `v0.1.0, v0.1.6, v0.1.7`.
8. **Must-not-forget identifiers:** `sdkModulePath = "github.com/mcpcat/mcpcat-go-sdk"`
   (`internal/diagnostics/constants.go:24`) is used for **self-version lookup** — if it doesn't
   match the new module path, version resolution silently breaks. Log file `~/mcpcat.log`
   (`internal/logging/log.go:136`) hardcoded. Diagnostics endpoint **already** `otel.agentcat.com`.
9. **Remote casing mismatch:** SDK remote is `MCPCat/…`, go-api is `mcpcat/…` (the rename
   normalizes both to `agentcathq`; §2A L124 notes this).

---

## E. CORRECT & well-handled (research + code confirm — keep as-is)

- Leave old `go.mod` **un-edited & un-retracted** — this is exactly the precondition that keeps
  both proxy and direct fetches working.
- **New path = new module @ v1.0.0**; old/new coexist; consumers migrate, not upgrade.
- **No-retract** decision itself (only its *rationale* needs the C3 fix).
- **Never recreate a repo at an old name**; publish new modules **with/after** the Phase-5 rename;
  stage as drafts, don't tag early.
- pkg.go.dev/GoReportCard don't follow redirects → re-badge (with M6 addition).
- README banner + `MIGRATION.md` as the deprecation surface. *(Decline the research's optional
  "final old-path version with `// Deprecated:` go.mod comment" — it conflicts with M1.)*
- `+incompatible`/v2 SIV concerns correctly absent (irrelevant to v1.0.0-at-new-path).

---

## F. Prioritized action list
1. **C1 + C2** — rewrite the refuted `GOPROXY=direct` caveat everywhere; append changelog reversal.
2. **M1, M2** — add "never re-tag old repos" + name the real error; lock in "new repos, don't rename old repos."
3. **M3** — add LICENSE-detectability rule; **add a LICENSE to `mcpcat-go-api`**.
4. **C3** — fix retract rationale.
5. **M4, O1(§B soften "permanently")** — never-delete rule; conditional-permanence wording.
6. **M5, M6, M7** — submodule tags, pkg.go.dev warm-up, vanity-path decision.
7. **D (ground truth)** — correct counts/line-numbers/literals in §3.5/§3.6; reconcile the
   already-scaffolded `agentcat-go-api`; ensure `sdkModulePath` is on the rename checklist.

*Note: the plan's own prior adversarial-review rounds introduced C1 by "correcting" accurate
text. The empirical test in §A is the tiebreaker — trust live toolchain output over issue-thread
inference.*
