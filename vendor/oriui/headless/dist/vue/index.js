import { a as connect, d as createNormalizer, n as connect$1, o as machine, r as machine$1 } from "../core-j3tsCnai.js";
import { computed, inject, onMounted, onUnmounted, provide, ref, shallowRef, toValue, useId, watch } from "vue";
//#region src/vue/contract.ts
/** Injection key the resolver reads; set by the OriHeadless plugin / provideHeadless(). */
const ORI_HEADLESS = Symbol("ori-headless");
//#endregion
//#region src/vue/normalize-props.ts
const propMap = {
	className: "class",
	htmlFor: "for",
	defaultValue: "value",
	defaultChecked: "checked"
};
const normalizeProps = createNormalizer((props) => {
	const result = {};
	for (const key in props) {
		const value = props[key];
		if (value === void 0) continue;
		result[propMap[key] ?? key] = value;
	}
	return result;
});
//#endregion
//#region src/vue/use-machine.ts
/**
* Bridge a core Service's `subscribe()` to Vue reactivity. Returns a version ref that bumps on
* every machine change; read it inside a `computed(() => connect(service, normalizeProps))` so the
* computed re-evaluates and the template re-binds. Subscription starts on mount (SSR-safe — the
* server render uses the machine's initial state, which matches the first client render).
*/
function useService(service) {
	const version = shallowRef(0);
	let unsubscribe;
	onMounted(() => {
		unsubscribe = service.subscribe(() => {
			version.value++;
		});
	});
	onUnmounted(() => unsubscribe?.());
	return version;
}
//#endregion
//#region src/vue/native.ts
/**
* Native oriUI Disclosure adapter — built on the in-house `../core` machine. The default behind
* `useDisclosure`; the contract still lets an app swap in a custom (e.g. Zag-backed) adapter.
*/
const nativeDisclosure = (options = () => ({})) => {
	const initial = toValue(options);
	const service = machine({
		id: initial.id ?? useId() ?? "disclosure",
		defaultOpen: initial.defaultOpen,
		disabled: initial.disabled
	});
	const version = useService(service);
	const api = computed(() => {
		version.value;
		return connect(service, normalizeProps);
	});
	return {
		open: computed(() => api.value.open),
		rootProps: computed(() => api.value.getRootProps()),
		triggerProps: computed(() => api.value.getTriggerProps()),
		contentProps: computed(() => api.value.getContentProps()),
		setOpen: (open) => service.send({
			type: "SET",
			open
		}),
		toggle: () => service.send({ type: "TOGGLE" })
	};
};
/**
* Native oriUI Dialog adapter — zero dependencies, built on the platform `<dialog>` element. It owns
* only the open state and the ARIA prop bags; the consuming component renders the `<dialog>` and calls
* `showModal()` / `close()` from `open` (see `OriDialog`), so the focus trap, `Esc`, `::backdrop`,
* top-layer and `inert`-on-rest come from the browser — the hard behaviour that previously justified a
* Zag adapter. This is the default behind `useDialog`; the `OriHeadless` contract still lets an app
* swap in a custom (e.g. Zag-backed) dialog adapter per project.
*/
const nativeDialog = (options = () => ({})) => {
	const opts = computed(() => toValue(options) ?? {});
	const baseId = opts.value.id ?? useId() ?? "ori-dialog";
	const titleId = `${baseId}-title`;
	const descriptionId = `${baseId}-description`;
	const open = ref(opts.value.defaultOpen ?? false);
	function setOpen(value) {
		if (open.value === value) return;
		open.value = value;
		opts.value.onOpenChange?.(value);
	}
	return {
		open: computed(() => open.value),
		setOpen,
		toggle: () => setOpen(!open.value),
		triggerProps: computed(() => ({
			"aria-haspopup": "dialog",
			"aria-expanded": open.value,
			onClick: () => setOpen(true)
		})),
		dialogProps: computed(() => ({
			role: "dialog",
			"aria-modal": opts.value.modal === false ? void 0 : "true",
			"aria-labelledby": titleId,
			onClose: () => setOpen(false),
			onCancel: opts.value.closeOnEscape === false ? (event) => event.preventDefault() : void 0,
			onClick: opts.value.closeOnInteractOutside === false ? void 0 : (event) => {
				if (event.currentTarget === event.target) setOpen(false);
			}
		})),
		titleProps: computed(() => ({ id: titleId })),
		descriptionProps: computed(() => ({ id: descriptionId })),
		closeTriggerProps: computed(() => ({ onClick: () => setOpen(false) }))
	};
};
//#endregion
//#region src/vue/use-disclosure.ts
/**
* Resolve the active Disclosure behavior. Components call this and stay engine-agnostic: it
* returns whichever adapter the app provided (Zag / custom) via the OriHeadless plugin, falling
* back to the native `../core` adapter when none is configured.
*/
function useDisclosure(options) {
	return (inject(ORI_HEADLESS, null)?.disclosure ?? nativeDisclosure)(options);
}
//#endregion
//#region src/vue/use-dialog.ts
/**
* Resolve the active Dialog behavior. Components call this and stay engine-agnostic: it returns
* whichever adapter the app provided (custom / Zag) via the OriHeadless plugin, falling back to the
* native `<dialog>`-backed adapter when none is configured. The native default gives the focus trap,
* `Esc`, `::backdrop`, top-layer and focus-return for free (`showModal()`), so a dialog needs no
* extra dependency — Zag is an optional per-widget swap, not a requirement.
*/
function useDialog(options) {
	return (inject(ORI_HEADLESS, null)?.dialog ?? nativeDialog)(options);
}
//#endregion
//#region src/vue/use-combobox.ts
const defaultFilter = (item, query) => item.label.toLowerCase().includes(query.trim().toLowerCase());
/**
* Headless single-select combobox — the first behavior to fully exercise the `@oriui/headless` core
* (state machine + prop-getters + WAI-ARIA listbox keyboard). Consumes the core engine directly and
* returns ready-to-`v-bind` prop bags plus the visible (filtered) items. Build any UI on top, or use
* the styled `OriCombobox`.
*/
function useCombobox(options) {
	const opts = computed(() => toValue(options));
	const init = opts.value;
	const service = machine$1({
		id: init.id ?? useId() ?? "combobox",
		defaultValue: init.value ?? null,
		defaultInputValue: init.inputValue ?? "",
		disabled: init.disabled
	});
	const version = useService(service);
	watch(() => opts.value.disabled ?? false, (disabled) => service.send({
		type: "SET_DISABLED",
		disabled
	}));
	const items = computed(() => {
		version.value;
		const { inputValue, value } = service.getState();
		const all = opts.value.options;
		const selectedLabel = value !== null ? all.find((option) => option.value === value)?.label : void 0;
		if (inputValue.trim() === "" || inputValue === selectedLabel) return all;
		const filter = opts.value.filter ?? defaultFilter;
		return all.filter((item) => filter(item, inputValue));
	});
	const api = computed(() => {
		version.value;
		return connect$1(service, normalizeProps, items.value);
	});
	return {
		open: computed(() => api.value.open),
		value: computed(() => api.value.value),
		inputValue: computed(() => api.value.inputValue),
		highlightedValue: computed(() => api.value.highlightedValue),
		items,
		rootProps: computed(() => api.value.getRootProps()),
		labelProps: computed(() => api.value.getLabelProps()),
		controlProps: computed(() => api.value.getControlProps()),
		inputProps: computed(() => api.value.getInputProps()),
		triggerProps: computed(() => api.value.getTriggerProps()),
		clearTriggerProps: computed(() => api.value.getClearTriggerProps()),
		listboxProps: computed(() => api.value.getListboxProps()),
		getOptionProps: (item, index) => api.value.getOptionProps(item, index),
		getOptionState: (item) => api.value.getOptionState(item),
		setOpen: (open) => api.value.setOpen(open),
		setInputValue: (next) => api.value.setInputValue(next),
		select: (item) => api.value.select(item),
		clear: () => api.value.clear()
	};
}
//#endregion
//#region src/vue/plugin.ts
/** Provide headless adapters to a component subtree (call inside `setup`). */
function provideHeadless(adapters) {
	provide(ORI_HEADLESS, adapters);
}
/**
* App-level plugin to choose the headless engine for the whole app:
* `app.use(OriHeadless, { disclosure: zagDisclosure })`. With no adapter the native one is used.
*/
const OriHeadless = { install(app, adapters = {}) {
	app.provide(ORI_HEADLESS, adapters);
} };
//#endregion
export { ORI_HEADLESS, OriHeadless, nativeDialog, nativeDisclosure, normalizeProps, provideHeadless, useCombobox, useDialog, useDisclosure, useService };

