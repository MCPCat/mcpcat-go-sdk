# Deep Research: Renaming the GitHub org for the Go module ecosystem

> **Scope:** Renaming GitHub org `mcpcat` → `agentcathq` (literal org rename, redirects on),
> while preserving resolution for consumers who pinned `github.com/mcpcat/mcpcat-go-sdk`
> (+ submodules `mcpgo`/`officialsdk`) and `github.com/mcpcat/mcpcat-go-api`, and publishing
> new modules at `github.com/agentcathq/agentcat-go-sdk` @ v1.0.0 after the rename.
>
> **Method:** fan-out web search → fetch ~16 sources → extract falsifiable claims → 3-vote
> adversarial verification (2/3 refutes kills a claim) → synthesize. 75 claims verified:
> **60 upheld, 15 refuted.** (The workflow's final auto-synthesis step crashed on a schema
> retry cap; this report is reconstructed from the completed, cached verification results.)

---

## TL;DR — your D16 plan is sound, but one of its load-bearing caveats is wrong

**Verdict on the plan (rename org, publish new modules at new path post-rename, leave old
modules un-retracted): CONFIRMED as the correct approach.** Every mechanism it relies on is
backed by primary Go sources.

**The one correction — and it's in your favor:** D16's "honest Go caveat" claims that a cold
`GOPROXY=direct` fetch of the *old* import path **fails after the rename because GitHub's
redirect does not preserve the Go module path.** **Adversarial verification refuted this claim
on every attempt (5 separate refutations, high confidence), including an empirical test.** Git
*does* follow GitHub's org-rename redirects, and because your plan leaves the old `go.mod`
un-edited, the declared module path still matches the requested path — so the direct fetch
**succeeds**. The "narrow accepted exception" in D16 appears to be **unnecessary pessimism**,
not a real cost. See §1 and §6.

There *is* a real hard-failure mode after renames — but it is **not** the one D16 names, and
your plan already avoids it. See §3.

---

## 1. How Go module fetches behave after the org rename

Your three sub-cases, resolved against primary sources:

**(a) Default `GOPROXY` (proxy.golang.org), old path, already-published version → WORKS.**
Module versions are immutable and the mirror caches content specifically "to avoid breaking
builds for people that depend on your package," continuing to serve source "that is no longer
available from the original locations." A pinned `github.com/mcpcat/...@v0.3.0` keeps
resolving from the proxy regardless of the rename.
*Sources: go.dev/blog/module-mirror-launch; proxy.golang.org FAQ; golang/go#37079 (a deleted-origin
module's specific `@v/<version>.info` still served by the proxy). [upheld, high]*

**(b) `GOPROXY=direct`, old path, already-published version → ALSO WORKS (contra D16).**
`GOPROXY=direct` makes `go` derive the repo URL from the module path and `git clone`/`ls-remote`
it. GitHub's rename redirect *is* followed by git, and since the old tags' `go.mod` still
declares `module github.com/mcpcat/mcpcat-go-sdk`, it matches the requested path and the build
succeeds. Verifiers reproduced this empirically on go1.24.4 (`git ls-remote` of a renamed repo
returned refs, exit 0; `GOPROXY=direct go mod download` of a renamed old path succeeded).
*Sources: GitHub Docs "Renaming a repository" + GitHub Blog "Repository redirects are here!"
(git clone/fetch/push follow rename redirects); go.dev/ref/mod (direct-mode VCS derivation);
empirical go1.24.4 test. **This directly refutes the D16 caveat.** [the D16 mechanism: refuted ×5, high]*

**(c) Anyone fetching a *new* version at the old path → correctly unavailable.**
You will publish new versions only at the new path. No new tags appear at the old path, so
there is simply nothing newer to resolve there — expected and harmless.

> **Caveat on (b) — the precondition that makes it true:** the direct-fetch success depends on
> the old repo's `go.mod` being left **un-edited** (path stays `github.com/mcpcat/...`) **and**
> the old repo never being deleted or recreated. Both are already in your plan. If you ever
> edited the old `go.mod` to the new path, or pushed a tag whose `go.mod` declares a different
> path, you would *create* the mismatch failure in §3.

---

## 2. Proxy cache immutability & the checksum database

- **Versions are immutable; `go.sum` entries are not invalidated by the rename.** Content is
  versioned together and "the contents of each version are immutable," authenticated via
  sum.golang.org. Old pins keep verifying. *[upheld, high]*
- **The mirror persists content independently of the origin.** It "caches metadata and source
  code in its own storage system," serving code no longer available upstream. *[upheld, high]*
- **⚠️ Correction to "the proxy keeps versions forever":** proxy.golang.org **explicitly states
  it "does not save all modules forever."** The documented eviction triggers are (i) **no
  detectable license** → only a *temporarily* cached copy that "may become unavailable," and
  (ii) a security/DMCA removal path. The "caches won't ever be removed" line that circulates is
  a *forum paraphrase* (golang/go#42433 reporter), **not** Go-team policy.
  *Sources: proxy.golang.org; golang/go#46440 & #49056 (removal requests — note #46440 was closed
  **without** removal: "this isn't likely to happen," recommending "tag a newer version"). [the
  "forever" absolute: refuted ×4]*
  - **Practical impact for you: low.** Your modules are **MIT-licensed**, so the license-detection
    eviction trigger does not apply, and the proxy's stated goal is precisely to keep licensed,
    depended-upon versions cached. Treat permanence as *strong in practice, not an absolute
    guarantee* — and **keep a detectable `LICENSE` file in the old repos** so the cache stays warm.

---

## 3. The module-path change & `retract` — what's actually true

- **Changing the module path = a brand-new module.** "Given that the module path is the module's
  identifier, this change effectively creates a new module"; users must **migrate** (update
  imports), not merely upgrade; the new path and old path **coexist** in the same build graph.
  Starting the new path at **v1.0.0** is correct. *Sources: go.dev/doc/modules/release-workflow,
  /major-version, /blog/v2-go-modules. [upheld ×9, high]*
- **`retract` does NOT delete and does NOT break existing builds** — it hides versions from
  `go list -m -versions`, excludes them from `@latest`/range queries, and notifies users on
  `go get -u`. Retracted versions "should remain available... so that builds that depend on them
  are not broken." *Source: go.dev/ref/mod#go-mod-file-retract. [upheld ×many, high]*
  - **Implication for your no-retract decision:** retracting wouldn't *break* pinned consumers
    (builds still work), but it's also **unnecessary and slightly counterproductive** here — a
    retract is a "don't use this version" signal, whereas your old versions remain perfectly
    good. Leaving them un-retracted with a README/`MIGRATION.md` pointer is the right call.
    *(Optional nuance: you could publish one final old-path version whose `go.mod` adds a
    `// Deprecated:` module comment — surfaces in tooling without retracting. Not required.)*

- **🔴 The REAL hard-failure mode after a rename (the one D16 should name instead of the
  redirect myth):** cmd/go aborts with
  **`module declares its path as: X but was required as: Y`** whenever a fetched `go.mod`'s
  `module` directive disagrees with the path it was required as. This string is live in the
  toolchain source (`cmd/go/internal/modload/modfile.go`) and is the documented failure across
  *every* real-world rename case the research found: `github.com/golang/lint`→`golang.org/x/lint`,
  `github.com/coreos/bbolt`→`go.etcd.io/bbolt`, `googleapis/gnostic`→`google/gnostic`,
  `4kills/go-libdeflate`→`libdeflate`. *Sources: go.dev/wiki/Resolving-Problems-From-Modified-Module-Path;
  golang/go#41350, #42433; cmd/go source. [upheld ×many, high]*
  - **You avoid this** precisely by (1) leaving the old `go.mod` un-edited and (2) publishing the
    new path as a separate new module. The mismatch only bites people who edit `go.mod` in place
    and re-tag, or who `require` the old path against tags that now declare a new path.
  - **One residual edge to know about (`go list -u -m all`):** golang/go#41350 shows that an
    *upgrade scan* over a dependency still pinned to a renamed old path can hit the mismatch error
    *while scanning for newer versions/retractions*. In your setup this stays safe **as long as
    you never push new tags to the old repo** — the old path only ever has its old, path-matching
    tags, so `-u` finds nothing inconsistent. Don't tag the old repos after the rename.

---

## 4. Nested submodules (`mcpgo`, `officialsdk`, examples) — guidance, lightly evidenced

The verification corpus was thin on multi-module-repo specifics (this is the **weakest-evidenced
section** — treat as best-practice, not verified-from-primary-source):

- Each nested module has its **own `go.mod`** and is an independent module path
  (`github.com/agentcathq/agentcat-go-sdk/mcpgo`, etc.). Each is versioned by its **own tag prefix**
  (`mcpgo/v1.0.0`), so each needs the same path rewrite + a fresh v1.0.0 tag at the new path.
- **`replace` directives in the `examples/*` modules** are local/dev-time only and are *ignored*
  by consumers who import your library, so they won't affect downstream resolution — but rewrite
  them to the new paths for internal consistency and so the examples build.
- Rewrite all 7 `go.mod` files + internal imports in one branch; tag the root and each submodule
  at the new path. The old repo's submodule tags stay un-edited (same as the root).

---

## 5. pkg.go.dev & GoReportCard across the rename

- **pkg.go.dev does not auto-follow GitHub redirects to a new module path** — it indexes by
  module path, and the new path is a *new module* that must be requested/indexed fresh (visiting
  `pkg.go.dev/github.com/agentcathq/agentcat-go-sdk` triggers indexing once the version is
  fetchable via the proxy). The old path's page persists for the old versions. *[supported by
  pkg.go.dev/about + the "new path = new module" rule; not contradicted]*
- **GoReportCard** is keyed on the path you give it; re-run it against the new path. Badges that
  point at shields.io/GitHub follow the org redirect fine; badges/links that embed the **module
  path** must be updated by hand.
- **Action:** after publishing, warm both `pkg.go.dev` pages for the new paths and re-badge.

---

## 6. Gotchas & confirmations

- **✅ Never recreate a repo at an old name.** GitHub drops the redirect the moment a new repo
  occupies the old `owner/name`, which would break BOTH the web redirect and (per §1b) the
  direct-mode git fetch of old pins. *(GitHub Docs.)*
- **✅ Don't delete the old repos.** `@v/list` returns *not-found* once the origin is gone even
  though specific cached versions may still resolve via the proxy (golang/go#37079). Keep the old
  repos alive under the renamed org.
- **✅ Keep a detectable `LICENSE` in the old repos** (§2) so proxy caching isn't downgraded to
  "temporary."
- **❌ The `+incompatible` / v2+ SIV rules are not in play** for you: you're going to v1.0.0 at a
  new path, not v2+ of the same path. (The `go.mod has post-v0 module path ".../vN"` error from
  golang/go#33099 is a *semantic-import-versioning* bug class, **irrelevant** to an org rename —
  several claims trying to tie it to redirects were **refuted**.)
- **Vanity import paths — the strongest "did we overlook anything?" finding.** A vanity/custom
  module path (e.g. `go.agentcat.com/sdk` served via `<meta name="go-import">`) would have made
  *this entire class of problem* a non-event: the import path is decoupled from GitHub, so future
  org/host moves never touch consumers. **Not worth retrofitting now** (it's itself a one-time
  path change = new module), but **strongly consider adopting a vanity path for the new
  `agentcat` modules** so the *next* rename costs nothing. *Sources: sagikazarmark.hu vanity-import
  blog; n16f.net "Taking control of your Go module paths." [supporting, medium]*

---

## 7. Sequencing & rollback

**Recommended sequence (matches your Phase-5 plan):**
1. Stage the rename branch (7× `go.mod` + imports + endpoint defaults) as a draft; do **not**
   tag yet.
2. Rename the org `mcpcat` → `agentcathq` (off-hours). Verify web + `git ls-remote` redirects.
3. Create the new repos `agentcat-go-sdk` / `agentcat-go-api` (or push the renamed module to new
   repos), tag root + submodules at **v1.0.0** at the new paths.
4. Trigger proxy/pkg.go.dev indexing for the new paths; re-badge.
5. On the **old** repos (now under `agentcathq`): add README banner + `MIGRATION.md`, leave
   `go.mod` un-edited, **do not retract, do not re-tag, do not delete**.

**Rollback story:** the org rename is the only hard-to-reverse step, and it's reversible —
GitHub lets you rename back, and a rename does not delete content, stars, or issues. Because old
pins resolve from the **immutable proxy cache** independent of the origin (§1a/§2), even a botched
intermediate state doesn't break existing consumers' builds. The genuinely irreversible mistakes
are **deleting** an old repo or **recreating** a repo at an old name — avoid both and there is no
one-way door here.

---

## Appendix — what the adversarial pass killed (15 refuted claims)

1. **"GOPROXY=direct of the old path fails because the redirect doesn't preserve the module
   path"** — refuted ×5 (high). Git follows the redirect; failure, when it occurs, is the
   `go.mod` path-mismatch (§3), not a redirect problem. Several refutations note the supporting
   "quote" was fabricated or mis-sourced to golang/go#53411 (which is actually a transient
   pkg.go.dev *indexing* issue, closed not-planned).
2. **"The proxy never removes cached versions / keeps them forever"** — refuted ×4. Contradicted
   by proxy.golang.org's own "does not save all modules forever"; the "forever" line is a forum
   paraphrase.
3. **golang/go#33099 "proves proxy vs. direct validate paths differently"** — refuted ×6. #33099
   is a v2+ SIV regression (a fixed *bug*, CL 186237), not a designed proxy behavior and not an
   org-rename scenario; supporting quotes were explicitly speculative forum guesses.

---

## Sources (primary unless noted)
- go.dev/ref/mod (Go Modules Reference; `#go-mod-file-retract`, `#module-path`)
- go.dev/doc/modules/release-workflow · /major-version
- go.dev/blog/module-mirror-launch · /blog/v2-go-modules
- proxy.golang.org (FAQ / policy)
- go.dev/wiki/Resolving-Problems-From-Modified-Module-Path
- cmd/go source: `internal/modload/modfile.go`, `internal/modfetch/coderepo.go`
- GitHub Docs "Renaming a repository" · GitHub Blog "Repository redirects are here!"
- golang/go issues: #37079, #41350, #42433, #46440, #49056, #33099, #31428, #53411 (mixed; several cited *against* the refuted claims)
- pkg.go.dev/about
- Secondary/community: rodaine.com (Go module rename), sagikazarmark.hu & n16f.net (vanity paths), byteincrements.com (proxy/checksum explainer)
