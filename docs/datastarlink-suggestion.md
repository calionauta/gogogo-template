# datastar-lint: catching PatchElements errors

## Context

The Datastar breakage we keep hitting (this session and others) is **not**
about routing paths — it's about `PatchElements` / fragment targets. A patch
whose selector resolves to no single stable element surfaces at runtime as
`PATCH_TARGET_NO_ID` (or, worse, a silently blank re-render). The earlier
`docs/datastar-lint-suggestion.md` sketched a `PATCH_TARGET_NO_ID` rule from the
"path" angle; that framing was a red herring. The real, repeated failure mode
is **PatchElements with a selector that can't match one element**.

This note answers the open question: is it worth extending datastar-lint to
catch it?

## Recommendation: yes — but scope it narrowly

It's worth doing. The failure mode is real and recurs:

- `sdk.PatchElements(sse, fragment, sdk.WithSelector("#todo-list"))` where
  `#todo-list` is not an `id` on the rendered root, so the patch has nothing
  to anchor to;
- a `MergeFragments` / `PatchElements` call whose `WithSelector` / `WithSelectorID`
  is a bare tag or class (`.todo-list`, `div`) instead of `#id` or a unique
  `[attr=val]`;
- a fragment emitted with **no** selector at all (defaults to `body`, almost
  always a bug in practice).

Catching these at lint/CI time — before they show up as a blank screen in the
browser — pays for itself.

### Rule sketch

Flag `PatchElements` / `MergeFragments` (and the `RenderAndPatch` wrappers in
`internal/datastar`) when:

1. the target selector is a bare tag/class rather than `#id` or a unique
   `[attr=val]`; or
2. the selector has no trailing `#id`; or
3. a fragment is emitted with no selector.

### Caveats — don't over-build

- Static lint can't see the runtime DOM, so treat hits as **warnings**, not
  errors, and allow an inline `// datastar-lint:ignore` escape hatch.
- Dynamic ids and server-rendered lists will false-positive. Keep the rule
  heuristic and documented, not exhaustive.
- Leave *path* routing to the router. The earlier "path errors" framing was
  the wrong target; **PatchElements** is the one worth guarding.

## Suggested next step

Extend the `PATCH_TARGET_NO_ID` rule already outlined in
`docs/datastar-lint-suggestion.md` to also cover `PatchElements`, run it at
warn level, and wire it into the lint / pre-commit gate (`make check`).
