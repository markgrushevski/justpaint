# Design system — how justpaint consumes oriui

> **A usage contract, not a style guide.** justpaint's UI is built on **oriui** (`@oriui/{vue,css,headless}`,
> exact-pinned, lockstep). This doc pins the **rules for consuming it** so the app never re-implements what
> the library already owns. Same spirit as `DOCUMENT-FORMAT.md`: a small set of invariants everyone honors.
>
> **Status:** adopted 2026-07-09 (from the owner's review of the `/draw` chrome). Companion to
> `ARCHITECTURE.md` (boundaries), `REVIEW.md` (the per-change bar), `NOTES.md` (gotchas).
>
> **Read the oriui source, not `dist`.** The authority is the oriui repo checked out alongside this one —
> **`../vueinjar`** (`@oriui/{css,headless,vue}` under `packages/`, guides under `docs/content/guides/`) — and the
> published docs: <https://oriui.vercel.app/llms-full.txt> (everything in one file) + `/guides/{customization,theming,design-tokens}`.
> Reading `node_modules/**/dist` instead cost two wrong claims about the Button API (2026-07-09); don't repeat that.
> When this doc disagrees with that source, **the library wins — fix this doc.** If a needed component is genuinely
> missing, tell the owner (they maintain oriui) rather than only wrapping it here.

## 0. The one rule

**Rent the design system; don't re-implement it.** Every color, state, and variant oriui already models is
consumed via **props**, never re-derived in a `.vue`'s `<style>`. If you're writing `color-mix(… var(--ori-color-*) …)`
or a `--active`/`--accent` class, stop — oriui already has it. And **wrap, don't repeat**: when the app needs a
recurring shape oriui doesn't ship, build ONE thin justpaint component (`components/ui/`) over oriui, so a future change
is one file, not a scattered find-and-replace (that's why `IconButton`/`SegmentedControl` exist).

**The customization ladder** (oriui's own `/guides/customization`, safest → most manual): (1) **props** — `color` role
+ `variant` mapping (the WCAG-AA contrast guarantee lives here); (2) **global rebrand** — repoint the
`--ori-color-<role>-light` / `-dark` **source** tokens in an **unlayered** `:root` (oriui ships in `@layer`, so your
unlayered rule wins with no `!important`; never repoint the resolved `--ori-color-<role>` alias — it flattens dark mode);
(3) **per-instance escape hatch** — `--ori-color` / `--ori-color-on` (+ `--ori-color-text` for a non-fill variant) on the
element, for a colour that isn't one of the eight roles; (4) **theming / canvas** — `useTheme` / `useThemeColor` from
`@oriui/headless/vue`. **Never override `.ori-*` internals** — they break between versions; props + tokens are the
stable public API. (Adding a documented *utility* class like `.ori-button_icon` is fine — that's the sanctioned §3 path,
not an override.)

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

- **`IconButton`** — `OriButton` preset for icon-only toolbar actions: `icon`, `variant` (default `text`), `active`,
  `disabled`, `label` (a11y + `OriTooltip`). Centralizes the toolbar-chip look so every island matches and no view
  re-styles a `<button>`. A SELECTED/on toggle passes `color="primary"` + `active`; a PRIMARY action is a `fill`
  `OriButton`, not this.
- **`SegmentedControl`** — single-select segmented button group (the theme Light/Dark/Auto picker; reusable for any
  small settings pick). COMPOSES `.ori-join` (oriui's segment-joiner) + `OriButton` (selected = `fill`, others =
  `outline` — no hand-rolled `--active`/`color-mix`) and ADDS the single-select model + radiogroup a11y
  (`role="radiogroup"`/`radio`, `aria-checked`, roving-tabindex arrow keys). `OriRadioGroup` exists but is the wrong
  visual here (radio circles, not a segmented look).

Everything else is **oriui direct**: **content** → `OriCard` (the ResultReveal sides — winner = `tonal`/`primary`).
**Floating chrome** (toolbar / zoom / panel over the canvas) → **`OriSurface`** — oriui's elevation primitive
(alpha-11; `as`, `bordered` default `true`, `elevation` default `'lg'` → **`--ori-shadow-lg`**, `radius` default
`'lg'`, its DEFAULTS are exactly the old `.jp-float` island look). **`JpFloat` is deleted** — use `OriSurface`
directly, no wrapper needed. (The `.jp-float` CSS *class* still exists in `main.css`: `FloatingToolbar.vue` hasn't
migrated off it yet — §6.) **Modal dialogs** → `OriDialog` (native `<dialog>`: focus-trap, scroll-lock, Esc,
backdrop) — alpha-11 made it **controlled** (`open` prop + `update:open`/`close` emits, `v-model:open`);
**ConfirmDialog / ShortcutsDialog are migrated to it.** A bespoke overlay whose layout isn't a textbook card
(EmptyState, JudgingOverlay) stays an `OriSurface` with custom content — don't force it into `OriCard`.

## 5. Migration checklist (a change touching chrome)

- [ ] No raw `<button>` for an action → `OriButton`/`IconButton`.
- [ ] No `color-mix()` of a **brand** role in a component `<style>`; no `--active`/`--accent` class → `active` prop.
- [ ] No `opacity` disabled override → `disabled` prop.
- [ ] One icon component per cluster (`OriIcon`).
- [ ] Floating chrome → `OriSurface`; content card → `OriCard`.
- [ ] `npm run lint:all` (incl. contrast) + `npm run test:a11y` still green.

## 6. oriui capability map — read the source, don't assume gaps

Every "gap" I first assumed (from `dist`) turned out to already exist in the source — that IS the lesson of §0's
"read the source". Current status, so no one re-spawns these as wants:

- **Elevation** — `--ori-shadow-{sm,md,lg,ring}` tokens (theme-aware; `OriDialog`/`OriPopover` use `-lg`). Alpha-11
  shipped **`OriSurface`**, oriui's elevation primitive, whose defaults reproduce the old `.jp-float` island look —
  `JpFloat` is retired (§4) in favor of `OriSurface` used directly. Not a gap.
- **Segmented / single-select** — `OriJoin` collapses adjacent controls into one segmented unit; `OriRadioGroup` is a
  native single-select radiogroup. `SegmentedControl` (§4) composes `.ori-join` + `OriButton`. Not a gap.
- **Neutral glyph** — `surface`/`background` ARE neutral roles; `color="surface"` (its `-text` alias resolves to
  `--ori-color-on-surface`) is the intended neutral. No `neutral` role needed.
- **Modal dialogs** — `OriDialog` exists (native `<dialog>` + `showModal()`) and, as of alpha-11, is **controlled**
  (`open` prop + `update:open`/`close` emits, `v-model:open`) — ConfirmDialog / ShortcutsDialog are migrated to it
  (§4). No longer a gap or an upstream-blocked item.
- **Toolbar chrome** — alpha-11 also shipped **`OriToolbar`**. `FloatingToolbar.vue` hasn't migrated to it yet
  (pending an icon-artwork decision) — the one remaining migration, and the reason the `.jp-float` CSS class still
  lives in `main.css` even though the `JpFloat` *component* is gone.
- **`llms-full.txt`** — published (oriui.vercel.app + `/guides/*`), just not in the npm tarball. Read it.

**If something IS genuinely missing**, tell the owner (they maintain oriui) — but confirm against `../vueinjar`
first, never assume from `dist`.
