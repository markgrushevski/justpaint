# Design system — how justpaint consumes oriui

> **A usage contract, not a style guide.** justpaint's UI is built on **oriui** (`@oriui/{vue,css,headless}`,
> exact-pinned, lockstep). This doc pins the **rules for consuming it** so the app never re-implements what
> the library already owns. Same spirit as `DOCUMENT-FORMAT.md`: a small set of invariants everyone honors.
>
> **Status:** adopted 2026-07-09 (from the owner's review of the `/draw` chrome). Companion to
> `ARCHITECTURE.md` (boundaries), `REVIEW.md` (the per-change bar), `NOTES.md` (gotchas). When this
> disagrees with the oriui source in `node_modules/@oriui`, **the library wins — fix this doc.**

## 0. The one rule

**Rent the design system; don't re-implement it.** Every color, state, and variant oriui already models is
consumed via **props**, never re-derived in a `.vue`'s `<style>`. If you're writing `color-mix(… var(--ori-color-*) …)`
or a `--active`/`--accent` class, stop — oriui already has it.

## 1. Color — set once at the root, never in components

- The **entire palette is defined once** in `apps/web/src/main.css` (`:root` + `:root.ori-theme_dark`):
  `--ori-color-primary/secondary/surface/background/outline/danger/warn/success/info` (+ `-on-*`), light & dark.
  That is the **only** place brand color is chosen.
- **Components MUST NOT re-derive brand colors.** `background: color-mix(in srgb, var(--ori-color-primary) 18%, transparent)`
  is **banned** — it hand-copies `.ori-variant_tonal` / `[data-active]`. Pick a `variant` + `color` prop and the
  library computes every state (rest/hover/active/disabled) from `--ori-color`.
- Every oriui variant is pure token math off `--ori-color`:
  | variant | rest | `[data-active]` / hover |
  |---|---|---|
  | `fill` | `bg=--ori-color`, `text=--ori-color-on` | `bg = mix(--ori-color, #fff 15%)` |
  | `tonal` | `bg = mix(--ori-color, transparent 75%)`, `text=--ori-color-text` | `bg = mix(…, transparent 70%)` |
  | `outline` | `border=--ori-color-text`, transparent bg | `bg = mix(…, transparent 90%)` |
  | `text` | transparent, `text=--ori-color-text` | `bg = mix(…, transparent 90%)` |
  | `plain` | transparent, **opacity 0.5** | opacity 1 |
- **Allowed** local color use: neutral structural tokens (`--ori-color-outline` for a hairline, `--ori-color-surface`
  for a panel bg) and justpaint's own **non-brand** tokens (`--jp-desk`). Re-mixing a *brand* role is not.

## 2. Buttons — always `OriButton`, drive state with props

`OriButton` props (from `@oriui/vue` `ori-button.vue.d.ts`): `variant`, `color` (`ThemeColor`), `active`,
`disabled`, `loading`, `icon`, `iconPosition`, `radius`, `size`, `fluid`, `text`, `as`.

- **No raw `<button>` for an action.** Use `OriButton` (or the `IconButton` wrapper, §4). A raw `<button>` is only
  acceptable for a bespoke non-button control that oriui genuinely doesn't model.
- **Toggle state = the `active` prop.** `<OriButton :active="panelOpen" …>` → sets `[data-active]`, which the variant
  styles. **Never** a hand-rolled `--active` class that swaps `tonal`↔`fill` or re-mixes a color.
- **Disabled = the `disabled` prop.** Never an `opacity: 0.35` override. (oriui dims to `.45` + blocks pointer events.)
- **Loading = the `loading` prop** (spinner + `[aria-busy]`), not a manual spinner.
- **Variant ladder (semantics we commit to):**
  - `fill` — the **one** primary/confirming action of a surface (Save, Submit, Confirm, Play again).
  - `outline` — secondary neutral actions (Cancel, New/Load/Export, Log out).
  - `tonal` — grouped/segmented mid-emphasis (auth tabs, theme segmented).
  - `text` / `plain` — quiet, low-chrome, icon-only toolbar actions; `plain` is the ghost (50% until hover/active).
  - `active` overlays any of them for the toggled state.

## 3. Icon buttons — a circle/rounded-square is built in

- `OriButton` in **icon mode** (the `ori-button_icon` sizing — via the `icon` prop or the public class) renders a
  square of `--ori-size-action` with `radius`: `radius="rounded"` (the default) ⇒ **circle**; `radius="md"` ⇒ rounded
  square. There is **no** need to hand-roll a square `<button>` for an icon — that was a stale assumption in the old
  `/draw` chrome.
- **One icon set per surface.** `ToolIcon` (custom 24×24 stroke SVGs, zero-dep) is the app's icon set; toolbar/island
  icon buttons render it through `IconButton` (§4), so every glyph in a cluster is one size. `OriIcon` (mdi paths from
  `icons.ts`) is used only where an oriui component takes an `icon` **path** prop (drawer/dialog buttons). **Never mix
  `ToolIcon` and `OriIcon` in the same cluster** — that was the "icons look different sizes" bug (Save via `OriIcon`
  next to Layers/Help via `ToolIcon`).

## 4. justpaint UI primitives (thin wrappers, `apps/web/src/components/ui/`)

Build a justpaint component **only** where oriui has a genuine gap or we want a project default. Keep them thin.

- **`JpFloat`** — the elevated floating-surface island (toolbar / zoom / panel over the canvas). oriui has **no**
  elevation primitive (`OriCard` is a flat content card with `gap_xl` padding + header/title semantics — wrong for
  chrome). `JpFloat` = surface bg + hairline + `radius_lg` + drop shadow, tight padding. Replaces the ad-hoc `.jp-float`
  utility. *(Upstream candidate: an oriui `OriSheet`/`elevation` — see §6.)*
- **`IconButton`** — `OriButton` preset for icon-only toolbar actions: `icon`, `variant` (default `text`), `active`,
  `disabled`, `label` (a11y + `OriTooltip`). Centralizes the toolbar-chip look so every island matches and no view
  re-styles a `<button>`. A SELECTED/on toggle passes `color="primary"` + `active`; a PRIMARY action is a `fill`
  `OriButton`, not this.
- **`SegmentedControl`** — single-select segmented button group (the theme Light/Dark/Auto picker; reusable for any
  small settings pick). oriui has no segmented/radio-group primitive (`OriTabs` is a label-only view-switching tablist,
  `OriSwitch` is binary), so this fills the gap. Built on `OriButton` (selected = `fill`, others = `text` — no
  hand-rolled `--active`/`color-mix`), with `role="radiogroup"` + roving-tabindex arrow-key a11y. *(Upstream candidate
  — see §6.)*

Content containers are **not** a justpaint primitive: use **`OriCard`** directly (result reveal, empty state, judging
card, dialog bodies) — it already models header/title/subtitle/body/actions.

## 5. Migration checklist (a change touching chrome)

- [ ] No raw `<button>` for an action → `OriButton`/`IconButton`.
- [ ] No `color-mix()` of a **brand** role in a component `<style>`; no `--active`/`--accent` class → `active` prop.
- [ ] No `opacity` disabled override → `disabled` prop.
- [ ] One icon component per cluster (`OriIcon`).
- [ ] Floating chrome → `JpFloat`; content card → `OriCard`.
- [ ] `npm run lint:all` (incl. contrast) + `npm run test:a11y` still green.

## 6. Gaps to push upstream to oriui (owner's repo — spec, don't fork here)

oriui is consumed from npm; changes are the owner's, in the oriui repo. Track wants here:

- **Elevation / `OriSheet`** — a floating-surface primitive (shadow + tight padding, no header semantics). Would let
  `JpFloat` become a re-export instead of a bespoke component.
- **Neutral ghost color for icon buttons** — `text`/`plain` derive text from `--ori-color` (defaults to `primary`);
  there is no first-class *neutral* role, so a plain grey toolbar glyph uses `color="surface"` (whose `-text` token is
  `--ori-color-on-surface`). A `neutral` `ThemeColor` (or a documented pattern) would make this intent explicit.
- **Segmented / radio-group control** — no oriui primitive for a single-select segment group (theme picker, view
  toggles). `SegmentedControl` (§4) fills it justpaint-side; an oriui `OriSegmented`/`OriRadioGroup` would let it become
  a re-export.
- **`llms.txt` / `AGENTS.md`** — a one-page index (components, variants, `ThemeColor`/`RadiusSize` unions, the
  "colors at root only" rule). The `.d.ts` are great contracts but there's no discoverability entry point; its absence
  cost a round of wrong assumptions about the Button API.
