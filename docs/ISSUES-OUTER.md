# Known issues — upstream

Problems this app hits that a dependency has to fix — almost always
[oriui](https://github.com/markgrushevski/oriui). Each entry names the upstream id it waits on and the
local workaround it justifies, so nobody deletes the workaround as dead code. oriui reads this file as
its inbound queue. Problems we fix ourselves live in [ISSUES-INNER.md](ISSUES-INNER.md).

**How to use this file.** Newest entry first. Never work around an oriui gap by styling `.ori-*`
internals: wrap it in `apps/web/src/components/ui/`, record it here, and report it upstream
([DESIGN-SYSTEM.md](DESIGN-SYSTEM.md) §0). "Fixed upstream" is not "fixed here" — an entry stays until
the release carrying the fix is installed, then it is deleted.

Status: `confirmed` · `mitigated` (a local workaround exists) · `fixed upstream` (released or merged —
note which, then bump and delete).

**All three open entries are fixed on oriui `main` and close with its next release.** That release
renames the component API vocabulary, so migrate the call sites before bumping
([ROADMAP.md](ROADMAP.md), "Dependencies").

---

## JP-O-11 — `OriTabs` renders its fallback slot into every panel

`fixed upstream` (oriui `ORI-I-84`, on `main`, unreleased)

- **Where:** `apps/web/src/components/auth/AuthForm.vue` puts the fields in `OriTabs`'s `#default` slot,
  so the sign-in form exists twice in the DOM — two `.auth-form` nodes, four `<input>`s bound to the same
  refs. The hidden copy is `display: none`, so users and assistive tech never meet it, but `<label for>`
  resolves to the first copy in document order: the hidden one whenever *Register* is the open tab.
- **Upstream fix:** the fallback renders into the active panel only. No change needed here.
- **After the bump:** typed values survive a tab switch (the refs live outside the slot); focus does not.
  Check that once.

## JP-O-10 — `OriButton` dims the `loading` state like `disabled`

`fixed upstream` (oriui `ORI-I-97`, on `main`, unreleased) · `mitigated`

- **Where:** `loading` sets the native `disabled` attribute, so `.ori-button:disabled { opacity: .45 }`
  hits a busy button too — the "Thinking…" label on a fill-primary button measures **1.68:1** (light)
  and **2.30:1** (dark). A busy control is not an inactive one, so the WCAG exemption doesn't cover it.
- **Upstream fix:** both dim selectors skip `[aria-busy='true']`; worst reading afterwards is 4.91:1.
- **Workaround (keep until the bump):** `apps/web/src/components/GuessResult.vue` states the wait in
  full-ink body copy and never relies on the button label to say the app is working.

## JP-O-09 — `OriDialog` fades its whole body below AA

`fixed upstream` (oriui `ORI-I-85`, on `main`, unreleased)

- **Where:** `.ori-dialog__body { opacity: 0.85 }` wraps everything slotted into a dialog, controls
  included. In the light theme the sign-in button label composites to **4.00:1** and a field hint to
  **3.95:1** (AA needs 4.5:1). `ConfirmDialog`'s danger button is hit by the same fade.
- **Upstream fix:** the fade is removed.
- **No workaround,** because overriding `.ori-dialog__body` would be exactly the vendor restyling the
  design system forbids. As a result `npm run test:a11y` fails one test (the open sign-in dialog) until
  the bump. Don't allowlist it; it goes green on its own.
