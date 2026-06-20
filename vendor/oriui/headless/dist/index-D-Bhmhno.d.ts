//#region src/core/types.d.ts
/**
 * Framework-neutral prop dictionary produced by a `connect()` prop-getter and handed to a
 * per-framework `normalizeProps` before it is spread onto an element.
 */
type Dict = Record<string, unknown>;
/**
 * Branding interface: each framework adapter widens these to its own element/attribute types
 * (Vue's `HTMLAttributes`, Svelte's, React's). The core only relies on the shape, not the types.
 */
interface PropTypes {
  element: Record<string, unknown>;
  button: Record<string, unknown>;
}
/**
 * The seam every adapter implements: a per-element transform that maps the neutral prop dict to
 * a framework's binding convention (e.g. Svelte lowercases `onClick` -> `onclick`).
 */
interface NormalizeProps<T extends PropTypes = PropTypes> {
  element(props: Dict): T['element'];
  button(props: Dict): T['button'];
}
/**
 * Build a NormalizeProps from a single transform. Most frameworks use the same transform for
 * every element kind; the split exists only so adapters can refine the return types.
 */
declare function createNormalizer<T extends PropTypes = PropTypes>(transform: (props: Dict) => Dict): NormalizeProps<T>;
//#endregion
//#region src/core/anatomy.d.ts
/**
 * Anatomy — names the "parts" of a component and produces the `data-scope` / `data-part`
 * attributes (and matching CSS selectors) that hang styling + querying off each part.
 *
 * Design follows Zag.js's `@zag-js/anatomy` (MIT); implementation is our own.
 */
interface AnatomyPart {
  attrs: {
    'data-scope': string;
    'data-part': string;
  };
  selector: string;
}
interface Anatomy<Part extends string> {
  name: string;
  parts: readonly Part[];
  build(): Record<Part, AnatomyPart>;
}
declare function createAnatomy<const Parts extends readonly string[]>(name: string, parts: Parts): Anatomy<Parts[number]>;
//#endregion
//#region src/core/merge-props.d.ts
declare function mergeProps(...sources: Dict[]): Dict;
//#endregion
//#region src/core/scope.d.ts
/**
 * Scope — owns a component instance's deterministic element ids. Ids are derived from one base
 * `id` (which the framework adapter sources from an SSR-stable generator: Vue `useId()`,
 * Svelte `$props.id()`), so server and client markup match and multiple instances never collide.
 *
 * `getRootNode` is the seam for future DOM queries (focus management, shadow DOM); kept minimal now.
 */
interface Scope {
  id: string;
  getId(part: string): string;
  getRootNode(): Document | ShadowRoot;
}
declare function createScope(options: {
  id: string;
  getRootNode?: () => Document | ShadowRoot;
}): Scope;
//#endregion
//#region src/core/machine.d.ts
/**
 * A deliberately tiny state container: a pure `reducer` over a context value, plus a subscribe
 * seam. The framework adapters bridge `subscribe()` to their reactivity (Vue `shallowRef`,
 * Svelte `$state`) — that is the ONLY place per-framework code touches state.
 *
 * For the primitives we ship (disclosure / toggle / tabs) a reducer is enough; a full statechart
 * would be overkill. The shape mirrors Zag.js's service so a real `@zag-js/<x>` machine can later
 * be dropped behind the same `connect()` seam for genuinely complex widgets.
 */
interface MachineConfig<Context, Event> {
  initial: Context;
  /** Pure transition. Return the SAME reference when nothing changes to skip a notification. */
  reducer(context: Context, event: Event): Context;
}
interface Service<Context, Event> {
  scope: Scope;
  getState(): Context;
  send(event: Event): void;
  subscribe(listener: () => void): () => void;
}
declare function createMachine<Context, Event>(config: MachineConfig<Context, Event>, scope: Scope): Service<Context, Event>;
//#endregion
//#region src/core/disclosure/disclosure.types.d.ts
interface DisclosureContext {
  open: boolean;
  disabled: boolean;
}
type DisclosureEvent = {
  type: 'TOGGLE';
} | {
  type: 'OPEN';
} | {
  type: 'CLOSE';
} | {
  type: 'SET';
  open: boolean;
};
interface DisclosureProps {
  /** Stable, SSR-safe base id (from the adapter: Vue `useId()`, Svelte `$props.id()`). */
  id: string;
  /** Uncontrolled initial open state. */
  defaultOpen?: boolean;
  disabled?: boolean;
}
//#endregion
//#region src/core/disclosure/disclosure.machine.d.ts
type DisclosureService = Service<DisclosureContext, DisclosureEvent>;
declare function machine$1(props: DisclosureProps): DisclosureService;
//#endregion
//#region src/core/disclosure/disclosure.connect.d.ts
interface DisclosureApi<T extends PropTypes = PropTypes> {
  /** Whether the content is currently expanded. */
  open: boolean;
  setOpen(open: boolean): void;
  toggle(): void;
  getRootProps(): T['element'];
  getTriggerProps(): T['button'];
  getContentProps(): T['element'];
}
/**
 * Pure projection of machine state -> prop-getters. Carries the WAI-ARIA Disclosure wiring
 * (`aria-expanded`, `aria-controls`/`aria-labelledby`, real `hidden`) and `data-state` styling
 * hooks. The framework adapter supplies `normalize` and re-invokes this on every state change.
 */
declare function connect$1<T extends PropTypes>(service: DisclosureService, normalize: NormalizeProps<T>): DisclosureApi<T>;
//#endregion
//#region src/core/disclosure/disclosure.anatomy.d.ts
declare const anatomy$1: Anatomy<"root" | "trigger" | "content">;
declare namespace index_d_exports$1 {
  export { DisclosureApi, DisclosureContext, DisclosureEvent, DisclosureProps, DisclosureService, anatomy$1 as anatomy, connect$1 as connect, machine$1 as machine };
}
//#endregion
//#region src/core/combobox/combobox.types.d.ts
/** A single combobox option. `value` is the stable identity; `label` is what the input shows. */
interface ComboboxItem {
  value: string;
  label: string;
  disabled?: boolean;
}
interface ComboboxContext {
  /** Whether the listbox is open. */
  open: boolean;
  /** The selected option value (single-select), or null. */
  value: string | null;
  /** The current text in the input. */
  inputValue: string;
  /** The highlighted (active) option value for keyboard navigation, or null. */
  highlightedValue: string | null;
  disabled: boolean;
}
type ComboboxEvent = {
  type: 'OPEN';
} | {
  type: 'CLOSE';
} | {
  type: 'SET_INPUT';
  value: string;
} | {
  type: 'HIGHLIGHT';
  value: string | null;
} | {
  type: 'SELECT';
  value: string;
  label: string;
} | {
  type: 'CLEAR';
} | {
  type: 'SET_DISABLED';
  disabled: boolean;
};
interface ComboboxProps {
  /** Stable, SSR-safe base id (from the adapter: Vue `useId()`, Svelte `$props.id()`). */
  id: string;
  /** Uncontrolled initial selected value. */
  defaultValue?: string | null;
  /** Uncontrolled initial input text. */
  defaultInputValue?: string;
  disabled?: boolean;
}
//#endregion
//#region src/core/combobox/combobox.machine.d.ts
type ComboboxService = Service<ComboboxContext, ComboboxEvent>;
/**
 * The combobox state machine — open state + selected value + input text + the highlighted option.
 * Deliberately dumb about the option list: keyboard navigation (which option is "next") and filtering
 * are computed by the consumer (it knows the visible collection) and fed back as `HIGHLIGHT` / `SELECT`
 * events. That keeps the machine a tiny pure reducer, framework- and collection-agnostic.
 */
declare function machine(props: ComboboxProps): ComboboxService;
//#endregion
//#region src/core/combobox/combobox.connect.d.ts
interface ComboboxOptionState {
  highlighted: boolean;
  selected: boolean;
}
interface ComboboxApi<T extends PropTypes = PropTypes> {
  open: boolean;
  value: string | null;
  inputValue: string;
  highlightedValue: string | null;
  setOpen(open: boolean): void;
  setInputValue(value: string): void;
  select(item: ComboboxItem): void;
  clear(): void;
  getRootProps(): T['element'];
  getLabelProps(): T['element'];
  getControlProps(): T['element'];
  getInputProps(): T['element'];
  getTriggerProps(): T['button'];
  getClearTriggerProps(): T['button'];
  getListboxProps(): T['element'];
  getOptionProps(item: ComboboxItem, index: number): T['element'];
  getOptionState(item: ComboboxItem): ComboboxOptionState;
}
/**
 * Pure projection of machine state -> prop-getters, carrying the WAI-ARIA combobox (listbox-popup)
 * wiring: `role="combobox"` + `aria-expanded` / `aria-controls` / `aria-activedescendant` on the input,
 * `role="listbox"` + `role="option"` + `aria-selected` on the popup, plus the full keyboard contract
 * (Arrow/Home/End move the highlight, Enter selects, Escape closes). `collection` is the currently
 * visible (already-filtered) items, so navigation and the active-descendant id stay in sync with what
 * the user sees. The framework adapter supplies `normalize` and re-invokes this on every state change.
 */
declare function connect<T extends PropTypes>(service: ComboboxService, normalize: NormalizeProps<T>, collection: ComboboxItem[]): ComboboxApi<T>;
//#endregion
//#region src/core/combobox/combobox.anatomy.d.ts
declare const anatomy: Anatomy<"root" | "label" | "control" | "input" | "trigger" | "clearTrigger" | "listbox" | "option">;
declare namespace index_d_exports {
  export { ComboboxApi, ComboboxContext, ComboboxEvent, ComboboxItem, ComboboxOptionState, ComboboxProps, ComboboxService, anatomy, connect, machine };
}
//#endregion
export { createNormalizer as _, MachineConfig as a, Scope as c, Anatomy as d, AnatomyPart as f, PropTypes as g, NormalizeProps as h, index_d_exports$1 as i, createScope as l, Dict as m, ComboboxOptionState as n, Service as o, createAnatomy as p, ComboboxItem as r, createMachine as s, index_d_exports as t, mergeProps as u };
//# sourceMappingURL=index-D-Bhmhno.d.ts.map