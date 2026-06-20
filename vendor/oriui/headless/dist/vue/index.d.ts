import { g as PropTypes, h as NormalizeProps, n as ComboboxOptionState, o as Service, r as ComboboxItem } from "../index-D-Bhmhno.js";
import { App, ComputedRef, InjectionKey, MaybeRefOrGetter, ShallowRef } from "vue";

//#region src/vue/contract.d.ts
interface UseDisclosureOptions {
  id?: string;
  defaultOpen?: boolean;
  disabled?: boolean;
}
/** The shape a component consumes, regardless of which engine produced it. */
interface DisclosureControl {
  open: ComputedRef<boolean>;
  rootProps: ComputedRef<Record<string, unknown>>;
  triggerProps: ComputedRef<Record<string, unknown>>;
  contentProps: ComputedRef<Record<string, unknown>>;
  setOpen(open: boolean): void;
  toggle(): void;
}
/**
 * A headless behavior implementation. Swap freely: our native `../core` one, a Zag-backed
 * one, or a user-supplied one — the component markup never changes.
 */
type DisclosureAdapter = (options?: MaybeRefOrGetter<UseDisclosureOptions>) => DisclosureControl;
interface UseDialogOptions {
  id?: string;
  defaultOpen?: boolean;
  modal?: boolean;
  closeOnEscape?: boolean;
  closeOnInteractOutside?: boolean;
  onOpenChange?: (open: boolean) => void;
}
/**
 * A modal/dialog control built around the native `<dialog>` element: the component renders a real
 * `<dialog>` and drives `showModal()` / `close()` from `open`, so the platform supplies the focus
 * trap, `Esc`, `::backdrop`, top-layer and `inert`-on-rest. The adapter owns only the open state and
 * the ARIA prop bags — `dialogProps` carries the `<dialog>`'s own attributes plus the `close` /
 * `cancel` / backdrop-click handlers that keep `open` in sync.
 */
interface DialogControl {
  open: ComputedRef<boolean>;
  setOpen(open: boolean): void;
  toggle(): void;
  triggerProps: ComputedRef<Record<string, unknown>>;
  dialogProps: ComputedRef<Record<string, unknown>>;
  titleProps: ComputedRef<Record<string, unknown>>;
  descriptionProps: ComputedRef<Record<string, unknown>>;
  closeTriggerProps: ComputedRef<Record<string, unknown>>;
}
type DialogAdapter = (options?: MaybeRefOrGetter<UseDialogOptions>) => DialogControl;
interface HeadlessAdapters {
  disclosure?: DisclosureAdapter;
  dialog?: DialogAdapter;
}
/** Injection key the resolver reads; set by the OriHeadless plugin / provideHeadless(). */
declare const ORI_HEADLESS: InjectionKey<HeadlessAdapters>;
//#endregion
//#region src/vue/use-disclosure.d.ts
/**
 * Resolve the active Disclosure behavior. Components call this and stay engine-agnostic: it
 * returns whichever adapter the app provided (Zag / custom) via the OriHeadless plugin, falling
 * back to the native `../core` adapter when none is configured.
 */
declare function useDisclosure(options?: MaybeRefOrGetter<UseDisclosureOptions>): DisclosureControl;
//#endregion
//#region src/vue/use-dialog.d.ts
/**
 * Resolve the active Dialog behavior. Components call this and stay engine-agnostic: it returns
 * whichever adapter the app provided (custom / Zag) via the OriHeadless plugin, falling back to the
 * native `<dialog>`-backed adapter when none is configured. The native default gives the focus trap,
 * `Esc`, `::backdrop`, top-layer and focus-return for free (`showModal()`), so a dialog needs no
 * extra dependency — Zag is an optional per-widget swap, not a requirement.
 */
declare function useDialog(options?: MaybeRefOrGetter<UseDialogOptions>): DialogControl;
//#endregion
//#region src/vue/use-combobox.d.ts
interface UseComboboxOptions {
  /** Stable base id; auto-generated via `useId` when omitted. */
  id?: string;
  /** The full option list. Reactive — filtering re-runs when it changes. */
  options: ComboboxItem[];
  /** Initial selected value. */
  value?: string | null;
  /** Initial input text. */
  inputValue?: string;
  disabled?: boolean;
  /** Filter predicate; default = case-insensitive substring on the label. */
  filter?: (item: ComboboxItem, query: string) => boolean;
}
/**
 * Headless single-select combobox — the first behavior to fully exercise the `@oriui/headless` core
 * (state machine + prop-getters + WAI-ARIA listbox keyboard). Consumes the core engine directly and
 * returns ready-to-`v-bind` prop bags plus the visible (filtered) items. Build any UI on top, or use
 * the styled `OriCombobox`.
 */
declare function useCombobox(options: MaybeRefOrGetter<UseComboboxOptions>): {
  open: import("vue").ComputedRef<boolean>;
  value: import("vue").ComputedRef<string | null>;
  inputValue: import("vue").ComputedRef<string>;
  highlightedValue: import("vue").ComputedRef<string | null>;
  items: import("vue").ComputedRef<ComboboxItem[]>;
  rootProps: import("vue").ComputedRef<Record<string, unknown>>;
  labelProps: import("vue").ComputedRef<Record<string, unknown>>;
  controlProps: import("vue").ComputedRef<Record<string, unknown>>;
  inputProps: import("vue").ComputedRef<Record<string, unknown>>;
  triggerProps: import("vue").ComputedRef<Record<string, unknown>>;
  clearTriggerProps: import("vue").ComputedRef<Record<string, unknown>>;
  listboxProps: import("vue").ComputedRef<Record<string, unknown>>;
  getOptionProps: (item: ComboboxItem, index: number) => Record<string, unknown>;
  getOptionState: (item: ComboboxItem) => ComboboxOptionState;
  setOpen: (open: boolean) => void;
  setInputValue: (next: string) => void;
  select: (item: ComboboxItem) => void;
  clear: () => void;
};
//#endregion
//#region src/vue/plugin.d.ts
/** Provide headless adapters to a component subtree (call inside `setup`). */
declare function provideHeadless(adapters: HeadlessAdapters): void;
/**
 * App-level plugin to choose the headless engine for the whole app:
 * `app.use(OriHeadless, { disclosure: zagDisclosure })`. With no adapter the native one is used.
 */
declare const OriHeadless: {
  install(app: App, adapters?: HeadlessAdapters): void;
};
//#endregion
//#region src/vue/native.d.ts
/**
 * Native oriUI Disclosure adapter — built on the in-house `../core` machine. The default behind
 * `useDisclosure`; the contract still lets an app swap in a custom (e.g. Zag-backed) adapter.
 */
declare const nativeDisclosure: (options?: MaybeRefOrGetter<UseDisclosureOptions>) => DisclosureControl;
/**
 * Native oriUI Dialog adapter — zero dependencies, built on the platform `<dialog>` element. It owns
 * only the open state and the ARIA prop bags; the consuming component renders the `<dialog>` and calls
 * `showModal()` / `close()` from `open` (see `OriDialog`), so the focus trap, `Esc`, `::backdrop`,
 * top-layer and `inert`-on-rest come from the browser — the hard behaviour that previously justified a
 * Zag adapter. This is the default behind `useDialog`; the `OriHeadless` contract still lets an app
 * swap in a custom (e.g. Zag-backed) dialog adapter per project.
 */
declare const nativeDialog: (options?: MaybeRefOrGetter<UseDialogOptions>) => DialogControl;
//#endregion
//#region src/vue/use-machine.d.ts
/**
 * Bridge a core Service's `subscribe()` to Vue reactivity. Returns a version ref that bumps on
 * every machine change; read it inside a `computed(() => connect(service, normalizeProps))` so the
 * computed re-evaluates and the template re-binds. Subscription starts on mount (SSR-safe — the
 * server render uses the machine's initial state, which matches the first client render).
 */
declare function useService<Context, Event>(service: Service<Context, Event>): ShallowRef<number>;
//#endregion
//#region src/vue/normalize-props.d.ts
interface VuePropTypes extends PropTypes {
  element: Record<string, unknown>;
  button: Record<string, unknown>;
}
declare const normalizeProps: NormalizeProps<VuePropTypes>;
//#endregion
export { type ComboboxItem, type DialogAdapter, type DialogControl, type DisclosureAdapter, type DisclosureControl, type HeadlessAdapters, ORI_HEADLESS, OriHeadless, type UseComboboxOptions, type UseDialogOptions, type UseDisclosureOptions, type VuePropTypes, nativeDialog, nativeDisclosure, normalizeProps, provideHeadless, useCombobox, useDialog, useDisclosure, useService };
//# sourceMappingURL=index.d.ts.map