# Design system — how justpaint consumes oriui

> **A usage contract, not a style guide.** justpaint's UI is built on **oriui** (`@oriui/{vue,css,headless}`,
> exact-pinned, lockstep). This doc pins the **rules for consuming it** so the app never re-implements what
> the library already owns. Same spirit as `DOCUMENT-FORMAT.md`: a small set of invariants everyone honors.
>
> **Status:** adopted 2026-07-09. Companion to
> `ARCHITECTURE.md` (boundaries), `REVIEW.md` (the per-change bar), `NOTES.md` (gotchas).
>
> **oriui is a separate library**, maintained alongside this project and consumed as a pinned dependency
> (`@oriui/{vue,css,headless}`, currently `1.0.0-rc.21`, all three in lockstep).
>
> **Read the oriui source, not `dist`.** The authority is the oriui repo checked out alongside this one —
> **`../vueinjar`** (`@oriui/{css,headless,vue}` under `packages/`, guides under `docs/content/guides/`) — and the
> published docs: <https://oriui.vercel.app/llms-full.txt> (everything in one file) + `/guides/{customization,theming,design-tokens}`.
> Reading `node_modules/**/dist` instead cost two wrong claims about the Button API (2026-07-09); don't repeat that.
> When this doc disagrees with that source, **the library wins — fix this doc.** If a needed component is genuinely
> missing, it gets added to oriui upstream — report the gap in `docs/ISSUES-OUTER.md` rather than only wrapping it here.

## 0. The one rule

**Rent the design system; don't re-implement it.** Every color, state, and variant oriui already models is
consumed via **props**, never re-derived in a `.vue`'s `<style>`. If you're writing `color-mix(… var(--ori-color-*) …)`
or a `--active`/`--accent` class, stop — oriui already has it. And **wrap, don't repeat**: when the app needs a
recurring shape oriui doesn't ship, build ONE thin justpaint component (`components/ui/`) over oriui, so a future change
is one file, not a scattered find-and-replace (that's why `IconButton`/`SwatchPicker` exist).

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
  `--ori-color-primary/secondary/surface/background/outline/danger/warning/success/info` (+ `-on-*`), light & dark.
  That is the **only** place brand color is chosen.
- **White paper on a concrete-grey desk, under one accent.** Light: surface `#f6f5f3`, page background `#ffffff`,
  ink `#242322`. Dark: `#302e2b` / `#161514` / `#ecebe8`. The hairline is `#85827c` in both themes, and the desk behind
  the sheet `#e4e2de` / `#242321`. Plain paper (a drawing with no background) is `PAPER` (`#ffffff`) in
  `features/editor/useBackdrop.ts`, the judge's white: view-only, never exported or judged.
- **The canvas colour belongs to the drawing.** `/draw`'s menu offers light and dark papers and a custom colour
  (`features/draw/canvasColors.ts`); the choice is `doc.background`, saved and undoable (`Editor.setBackground`). A
  new drawing starts on the theme's paper, and an untouched one follows the theme. The pen's default ink flips with
  the paper's lightness unless the player picked a colour.
- **Inverting the canvas in the dark theme is the player's choice.** With "Invert in the dark theme" on, the canvas,
  the toolbar's colour wells, the canvas swatches and the gallery previews carry `.jp-ink-view`, which `main.css`
  inverts under `:root.jp-canvas-dark`, as Excalidraw does. Off (the default), colours show as they are. Exports and
  the judge always get the drawing as kept; the scored modes never invert.
- **The accent is the player's pick; orange is the default.** A preset is a `jp-accent-<name>` class on the root, and
  `main.css` gives each its four primary sources (fill and ink, light and dark), which `check-contrast.mjs` holds to
  the orange's bars. A custom colour goes inline on the root: `core/utils/color.ts` moves it just far enough to
  clear 3:1 against each page and picks black or white ink, which always clears 4.5:1. Class and style, never a data
  attribute: oriui's token observer (the cursor ring's `useThemeColor`) watches only those.
- **The wordmark sits on the page background, not the surface.** It is large text, and the orange clears the 3:1 bar
  only there (3.20:1 on `#ffffff`). `scripts/check-contrast.mjs` checks `primary-light` against
  `background-light`; `ModeNav` gives its island the page background for this reason.
- **One typeface: Nunito**, set in `main.css` and loaded in `index.html`. No hand-drawn or display second face
  (`docs/DECISIONS.md`, 2026-10-02).
- **Components MUST NOT re-derive brand colors.** `background: color-mix(in srgb, var(--ori-color-primary) 18%, transparent)`
  is **banned** — it hand-copies `.ori-variant_soft` / `[data-active]`. Pick a `variant` + `color` prop and the
  library computes every state (rest/hover/active/disabled) from `--ori-color`.
- Every oriui variant is pure token math off `--ori-color`:
  | variant | rest | `[data-active]` / hover |
  |---|---|---|
  | `solid` | `bg=--ori-color`, `text=--ori-color-on` | `bg = mix(--ori-color, #fff 15%)` |
  | `soft` | `bg = mix(--ori-color, transparent 75%)`, `text=--ori-color-text` | `bg = mix(…, transparent 70%)` |
  | `outline` | `border=--ori-color-text`, transparent bg | `bg = mix(…, transparent 90%)` |
  | `text` | transparent, `text=--ori-color-text` | `bg = mix(…, transparent 90%)` |
  | `quiet` | transparent, **opacity 0.85** | opacity 1 |
- **Allowed** local color use: neutral structural tokens (`--ori-color-surface` for a panel bg) and justpaint's own
  **non-brand** tokens (`--jp-desk`, `--jp-color-outline` for a hairline). Re-mixing a *brand* role is not.
  The hairline is deliberately **ours**, not oriui's `--ori-color-outline`: theirs is a `currentcolor` tint that
  cannot meet the 3:1 non-text bar `scripts/check-contrast.mjs` enforces — same name, different job
  ([ISSUES-OUTER.md](ISSUES-OUTER.md) JP-O-06).
- **More contrast when the system asks for it** (`prefers-contrast: more`). `main.css` darkens the hairline in both
  themes to clear 4.5:1 (checked by `check-contrast.mjs`), points oriui's `--ori-color-outline` and
  `--ori-color-outline-strong` at it, and sets `--jp-dim: 1`. Dimmed secondary text is written
  `opacity: var(--jp-dim, 0.7)` with its own number as the fallback, so it comes back to full strength; a decorative
  mark or a disabled state keeps a plain opacity. Islands take a hairline (§4, `IslandSurface`).
- **Forced colours** (Windows contrast themes) replace colours with system ones and drop shadows. A colour swatch is
  its colour, so `SwatchPicker`'s dots and the toolbar's mobile colour dot set `forced-color-adjust: none` and draw
  their rings as outlines in system colours. Islands take the hairline, which the mode draws. States oriui shows by
  fill alone (the pressed tool, the selected segment, the switch, the slider) are lost there for now
  ([ISSUES-OUTER.md](ISSUES-OUTER.md) JP-O-15).

## 2. Buttons — always `OriButton`, drive state with props

`OriButton` props (from `@oriui/vue` `ori-button.vue.d.ts`): `variant`, `color` (`ThemeColor`), `active`,
`pressed`, `disabled`, `loading`, `icon`, `iconPosition`, `radius`, `size`, `fluid`, `label`, `as`.

- **No raw `<button>` for an action.** Use `OriButton` (or the `IconButton` wrapper, §4). A raw `<button>` is only
  acceptable for a bespoke non-button control that oriui genuinely doesn't model.
- **Toggle state = the `active` prop.** `<OriButton :active="panelOpen" …>` → sets `[data-active]`, which the variant
  styles. **Never** a hand-rolled `--active` class that swaps `soft`↔`solid` or re-mixes a color.
- **Disabled = the `disabled` prop.** Never an `opacity: 0.35` override. (oriui dims to `.45` + blocks pointer events.)
- **Loading = the `loading` prop** (spinner + `[aria-busy]`), not a manual spinner.
- **Variant ladder (semantics we commit to):**
  - `solid` — the **one** primary/confirming action of a surface (Save, Submit, Confirm, Play again). `/draw`'s Save
    is `solid` only while there are unsaved changes and `soft` otherwise.
  - `color="danger"` — `solid` when the action destroys something saved (Delete), `outline` when it drops only unsaved
    work (Leave, Clear the canvas): `ConfirmDialog`'s `danger` and `discard`.
  - `outline` — secondary neutral actions (Cancel, Apply size, Log out).
  - `soft` — grouped/segmented mid-emphasis (auth tabs, theme segmented).
  - `text` / `quiet` — low-chrome, icon-only toolbar actions; `quiet` is the ghost (85% until hover/active).
  - `active` overlays any of them for the toggled state.

## 3. Icon buttons — a circle/rounded-square is built in

- `OriButton` in **icon mode** (the `ori-button_icon` sizing — via the `icon` prop or the public class) renders a
  square of `--ori-size-action` with `radius`: `radius="full"` (the default) ⇒ **circle**; `radius="md"` ⇒ rounded
  square. There is **no** need to hand-roll a square `<button>` for an icon — that was a stale assumption in the old
  `/draw` chrome.
- **One icon set per surface.** `ToolIcon` (custom 24×24 stroke SVGs, zero-dep) is the app's icon set; toolbar/island
  icon buttons render it through `IconButton` (§4), so every glyph in a cluster is one size. `OriIcon` (mdi paths from
  `icons.ts`) is used only where an oriui component takes an `icon` **path** prop, such as the rows of the `/draw`
  menu (`OriListItem`). **Never mix `ToolIcon` and `OriIcon` in the same cluster** — that was the "icons look different
  sizes" bug (Save via `OriIcon` next to Layers/Help via `ToolIcon`).

## 4. justpaint UI primitives (thin wrappers, `apps/web/src/components/ui/`)

Build a justpaint component **only** where oriui has a genuine gap or we want a project default. Keep them thin.

- **`IconButton`** — `OriButton` preset for icon-only toolbar actions: `icon`, `variant` (default `text`), `active`,
  `disabled`, `label` (a11y + `OriTooltip`). Centralizes the toolbar-chip look so every island matches and no view
  re-styles a `<button>`. A SELECTED/on toggle passes `color="primary"` + `active`; a PRIMARY action is a `solid`
  `OriButton`, not this. Its tooltip gets an anchor name of its own (`--ori-anchor`), and so does every
  `OriTooltip` we render: the shared default lets a bubble open at another trigger
  ([ISSUES-OUTER.md](ISSUES-OUTER.md) JP-O-14).
- **`IslandSurface`** — floating chrome over the canvas: `OriSurface` with no hairline, lifted by its shadow
  (`elevation`, default `md`; `as`). The hairline comes on when the system asks for more contrast or forces colours
  (`useThemeStore().moreContrast`), since a forced-colours mode drops the shadow.
- **`SwatchPicker`** — a single-select grid of colour dots (the accent, the canvas colour) with an optional custom
  dot that opens `OriColorPicker` in an `OriPopover`. Each preset is an icon-mode `OriButton` painted through the
  per-instance `--ori-color` / `--ori-color-on` escape hatch (§0), named by an `OriTooltip`, in a radiogroup with
  roving focus; the selection ring is a wrapper of ours, so the button stays oriui's.

The `/draw` menu's rows are `OriList` / `OriListItem` and its theme picker is `OriSegmentedControl`, icon-only with
the names kept for assistive technology (`.jp-sr-only`). On a wide screen the menu is a non-modal `IslandSurface`
under its toggle; at 600px and below it is a modal `OriDrawer` from the right edge, titled with the drawing's name.

Everything else is **oriui direct**: **content** → `OriCard` (the ResultReveal sides — winner = `soft`/`primary`;
the welcome's mode cards, which also carry `data-ori-interactive`, §7).
**Floating chrome** (toolbar / zoom / panel over the canvas) → **`IslandSurface`** (above), over oriui's
elevation primitive `OriSurface`. Islands carry no border unless more contrast is asked for: chrome on the canvas
edge takes `elevation="md"`, panels over content keep `lg`, and `/draw`'s Layers, AI and Save share one island so
the top row is one height. **Modal dialogs** → `OriDialog`
(native `<dialog>`: focus-trap, scroll-lock, Esc,
backdrop) — alpha-11 made it **controlled** (`open` prop + `update:open`/`close` emits, `v-model:open`);
**ConfirmDialog / ShortcutsDialog are migrated to it.** A bespoke overlay whose layout isn't a textbook card
(JudgingOverlay) stays an `IslandSurface` with custom content — don't force it into `OriCard`.

## 5. Migration checklist (a change touching chrome)

- [ ] No raw `<button>` for an action → `OriButton`/`IconButton`.
- [ ] No `color-mix()` of a **brand** role in a component `<style>`; no `--active`/`--accent` class → `active` prop.
- [ ] No `opacity` disabled override → `disabled` prop.
- [ ] One icon component per cluster — `ToolIcon` is the app's set (§3); `OriIcon` only where a path prop is passed.
- [ ] Floating chrome → `IslandSurface`; content card → `OriCard`.
- [ ] Dimmed text is `opacity: var(--jp-dim, <n>)`; anything that shows a colour still shows it in a forced-colours
      mode (§1).
- [ ] Motion sits on our own elements or glyphs, never on `.ori-*` or `--ori-variant-*`, and each animation has a
      `prefers-reduced-motion: reduce` rule (§7).
- [ ] `npm run lint:all` (incl. contrast) + `npm run test:a11y` + `npm run test:layout` still green — the last one whenever the change touches floating/absolute chrome.

## 6. oriui capability map — read the source, don't assume gaps

Every "gap" first assumed (from `dist`) turned out to already exist in the source — that IS the lesson of §0's
"read the source". Current status, so nobody re-opens these as wants:

- **Elevation** — `--ori-shadow-{sm,md,lg,ring}` tokens (theme-aware; `OriDialog`/`OriPopover` use `-lg`). Alpha-11
  shipped **`OriSurface`**, oriui's elevation primitive, whose defaults reproduce the old `.jp-float` island look —
  `JpFloat` is retired in favor of `OriSurface`, which `IslandSurface` wraps (§4). Not a gap.
- **Segmented / single-select** — `OriJoin` collapses adjacent controls into one segmented unit; `OriRadioGroup` is a
  native single-select radiogroup, and rc.21 shipped `OriSegmentedControl`. Not a gap.
- **Neutral glyph** — `surface`/`background` ARE neutral roles; `color="surface"` (its `-text` alias resolves to
  `--ori-color-on-surface`) is the intended neutral. No `neutral` role needed.
- **Modal dialogs** — `OriDialog` exists (native `<dialog>` + `showModal()`) and, as of alpha-11, is **controlled**
  (`open` prop + `update:open`/`close` emits, `v-model:open`) — ConfirmDialog / ShortcutsDialog are migrated to it
  (§4). No longer a gap or an upstream-blocked item.
- **Toolbar chrome** — alpha-11 shipped **`OriToolbar`**; **alpha-12** added a **content slot** on
  `OriToolbarButton` / `OriToolbarToggleItem`, so our multi-path `ToolIcon` slots straight in (keep the icon set,
  no headless `useToolbar`). **`FloatingToolbar.vue` is migrated** (2026-07-10): an island (`IslandSurface`) wrapping two
  `OriToolbar`s — the 7 tools as a single-select `OriToolbarToggleGroup` (parent owns `activeTool`, bound one-way with
  a guard that ignores the deselect-to-`undefined` a single group allows), undo/redo as `OriToolbarButton`s — with the
  stroke/fill form controls a plain group between them. This retired the **last** `.jp-float` user, so the CSS class is
  **deleted** from `main.css`. Item sizing uses `class="ori-button_icon"` (alpha-12's explicit icon-mode) exactly like
  `IconButton`; the mobile 32px shrink repoints `--ori-size-action` on the button (the §0 token escape-hatch). The
  active tool renders the intended **neutral 18% fill + inset ring** (OriToolbar's `[aria-pressed=true]`) with a
  **brand-tinted glyph** (`:color="active ? 'primary' : 'surface'"`); resting tools are the neutral `surface` glyph.
  Every item passes its own `aria-label`: oriui treats a filled content slot as a visible name, so an icon there
  would leave the button nameless and the tooltip only describes it ([ISSUES-OUTER.md](ISSUES-OUTER.md) JP-O-12).

  **Two oriui-side bugs this migration surfaced — both FIXED in alpha-13 (2026-07-10), kept here as the lesson:**
  - **(A) layer order beat specificity — the pressed FILL didn't land.** alpha-12's
    `.ori-toolbar .ori-button[aria-pressed=true]` set the fill via `--ori-variant-bg-color` (layer `ori.components`),
    but `.ori-variant_text` in the **later** layer `ori.utilities` re-set that token to `transparent` — a later layer
    beats specificity — so every variant-styled toggle item (all default `variant="text"`) lost its fill, showing the
    ring only. **alpha-13 fix:** the pressed rule now paints `background-color` **directly** (not via the token), so
    `.ori-variant_text`'s token no longer defeats it. (The same layer mechanic still works FOR us elsewhere: `main.css`'s
    unlayered `:where(button,…):focus-visible` outline beats oriui's layered one — see NOTES.)
  - **(B) `color` transitions couldn't interpolate relative-colour role tokens.** alpha-12's `.ori-button` had
    `transition: … color …`; the role text tokens are `oklch(from <color> …)` relative colours the browser can't
    interpolate, so a per-selection `color` swap left the glyph **stuck** until a repaint. **alpha-13 fix:** `color`
    was dropped from `.ori-button`'s transition (only `opacity` / `background-color` / `border-color` animate now), so
    a glyph swap is instant — which is what unblocked flipping the active item to `color="primary"` above.
- **`llms-full.txt`** — published (oriui.vercel.app + `/guides/*`), just not in the npm tarball. Read it.

**If something IS genuinely missing** (or broken, like A/B above), it gets added to oriui upstream — report the gap in
`docs/ISSUES-OUTER.md`, but confirm against `../vueinjar` first, never assume from `dist`.

## 7. Motion and hover — ours on ours

oriui owns how its components react. Hover and active tints come from the variant vocabulary (§1), so a
component's own hover is never restyled: not through `.ori-*` rules, and not by repointing its internal
`--ori-variant-*` variables.

What we may animate:

- **The round's two cards, and nothing else on its own.** The prompt card is dealt when a round starts and put away
  on the first stroke (`GamePromptBanner`); the score card is held up when the judge answers (`PracticeResult`,
  `ResultReveal`). Every other motion answers something the player did.
- **Our own elements** — the `ModeNav` underline drawing in under the current mode, the `/draw` menu card fading
  and scaling in.
- **Our glyphs inside an oriui component** — the `ToolIcon` in each `OriToolbar` item lifts on hover, squashes on
  press and bounces when its tool is picked. The transform is on the glyph; the button's background states stay
  oriui's.
- **A wrapper we own around an oriui component** — a gallery card lifts on a `div` around the `OriCard`, and the
  welcome's mode cards nudge on the link or button around them, so the card keeps its own transitions.

A block that isn't a button but should react like one, such as an `OriCard` used as a mode card, opts in with
`data-ori-interactive` (public oriui API since rc.18). The variant vocabulary's hover and active tints then apply
to it exactly as they do to an `OriButton`; don't rebuild them with `color-mix` (§1).

Every animation has a `prefers-reduced-motion: reduce` rule that turns it off.
