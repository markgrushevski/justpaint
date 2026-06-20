import { t as __exportAll } from "./rolldown-runtime-D7D4PA-g.js";
//#region src/core/types.ts
/**
* Build a NormalizeProps from a single transform. Most frameworks use the same transform for
* every element kind; the split exists only so adapters can refine the return types.
*/
function createNormalizer(transform) {
	return {
		element: transform,
		button: transform
	};
}
//#endregion
//#region src/core/anatomy.ts
function createAnatomy(name, parts) {
	return {
		name,
		parts,
		build() {
			const result = {};
			for (const part of parts) result[part] = {
				attrs: {
					"data-scope": name,
					"data-part": part
				},
				selector: `[data-scope="${name}"][data-part="${part}"]`
			};
			return result;
		}
	};
}
//#endregion
//#region src/core/merge-props.ts
const isFn = (value) => typeof value === "function";
const isHandler = (key) => key.length > 2 && key[0] === "o" && key[1] === "n";
function chain(...fns) {
	return (...args) => {
		for (const fn of fns) if (isFn(fn)) fn(...args);
	};
}
function mergeProps(...sources) {
	const result = {};
	for (const props of sources) for (const key in props) {
		const prev = result[key];
		const next = props[key];
		if (isHandler(key) && isFn(prev) && isFn(next)) result[key] = chain(prev, next);
		else if ((key === "class" || key === "className") && prev && next) result[key] = `${prev} ${next}`;
		else if (key === "style" && isObject(prev) && isObject(next)) result[key] = {
			...prev,
			...next
		};
		else result[key] = next !== void 0 ? next : prev;
	}
	return result;
}
function isObject(value) {
	return typeof value === "object" && value !== null;
}
//#endregion
//#region src/core/scope.ts
function createScope(options) {
	const { id, getRootNode } = options;
	return {
		id,
		getId: (part) => `ori-${id}-${part}`,
		getRootNode: getRootNode ?? (() => document)
	};
}
//#endregion
//#region src/core/machine.ts
function createMachine(config, scope) {
	let state = config.initial;
	const listeners = /* @__PURE__ */ new Set();
	return {
		scope,
		getState: () => state,
		send(event) {
			const next = config.reducer(state, event);
			if (next === state) return;
			state = next;
			for (const listener of listeners) listener();
		},
		subscribe(listener) {
			listeners.add(listener);
			return () => {
				listeners.delete(listener);
			};
		}
	};
}
//#endregion
//#region src/core/disclosure/disclosure.machine.ts
function machine$1(props) {
	const scope = createScope({ id: props.id });
	return createMachine({
		initial: {
			open: props.defaultOpen ?? false,
			disabled: props.disabled ?? false
		},
		reducer(context, event) {
			switch (event.type) {
				case "TOGGLE": return context.disabled ? context : {
					...context,
					open: !context.open
				};
				case "OPEN": return context.open ? context : {
					...context,
					open: true
				};
				case "CLOSE": return !context.open ? context : {
					...context,
					open: false
				};
				case "SET": return context.open === event.open ? context : {
					...context,
					open: event.open
				};
				default: return context;
			}
		}
	}, scope);
}
//#endregion
//#region src/core/disclosure/disclosure.anatomy.ts
const anatomy$1 = createAnatomy("disclosure", [
	"root",
	"trigger",
	"content"
]);
//#endregion
//#region src/core/disclosure/disclosure.connect.ts
const parts$1 = anatomy$1.build();
/**
* Pure projection of machine state -> prop-getters. Carries the WAI-ARIA Disclosure wiring
* (`aria-expanded`, `aria-controls`/`aria-labelledby`, real `hidden`) and `data-state` styling
* hooks. The framework adapter supplies `normalize` and re-invokes this on every state change.
*/
function connect$1(service, normalize) {
	const { open, disabled } = service.getState();
	const { scope } = service;
	const triggerId = scope.getId("trigger");
	const contentId = scope.getId("content");
	return {
		open,
		setOpen(next) {
			service.send({
				type: "SET",
				open: next
			});
		},
		toggle() {
			service.send({ type: "TOGGLE" });
		},
		getRootProps() {
			return normalize.element({
				...parts$1.root.attrs,
				"data-state": open ? "open" : "closed"
			});
		},
		getTriggerProps() {
			return normalize.button({
				...parts$1.trigger.attrs,
				id: triggerId,
				type: "button",
				"aria-controls": contentId,
				"aria-expanded": open,
				"data-state": open ? "open" : "closed",
				"data-disabled": disabled ? "" : void 0,
				disabled: disabled || void 0,
				onClick() {
					service.send({ type: "TOGGLE" });
				}
			});
		},
		getContentProps() {
			return normalize.element({
				...parts$1.content.attrs,
				id: contentId,
				role: "region",
				"aria-labelledby": triggerId,
				hidden: !open,
				"data-state": open ? "open" : "closed"
			});
		}
	};
}
//#endregion
//#region src/core/disclosure/index.ts
var disclosure_exports = /* @__PURE__ */ __exportAll({
	anatomy: () => anatomy$1,
	connect: () => connect$1,
	machine: () => machine$1
});
//#endregion
//#region src/core/combobox/combobox.machine.ts
/**
* The combobox state machine — open state + selected value + input text + the highlighted option.
* Deliberately dumb about the option list: keyboard navigation (which option is "next") and filtering
* are computed by the consumer (it knows the visible collection) and fed back as `HIGHLIGHT` / `SELECT`
* events. That keeps the machine a tiny pure reducer, framework- and collection-agnostic.
*/
function machine(props) {
	const scope = createScope({ id: props.id });
	return createMachine({
		initial: {
			open: false,
			value: props.defaultValue ?? null,
			inputValue: props.defaultInputValue ?? "",
			highlightedValue: null,
			disabled: props.disabled ?? false
		},
		reducer(context, event) {
			switch (event.type) {
				case "OPEN": return context.open ? context : {
					...context,
					open: true
				};
				case "CLOSE": return !context.open && context.highlightedValue === null ? context : {
					...context,
					open: false,
					highlightedValue: null
				};
				case "SET_INPUT": return {
					...context,
					inputValue: event.value,
					open: true,
					highlightedValue: null
				};
				case "HIGHLIGHT": return context.highlightedValue === event.value ? context : {
					...context,
					highlightedValue: event.value
				};
				case "SELECT": return {
					...context,
					value: event.value,
					inputValue: event.label,
					open: false,
					highlightedValue: null
				};
				case "CLEAR": return context.value === null && context.inputValue === "" ? context : {
					...context,
					value: null,
					inputValue: "",
					highlightedValue: null
				};
				case "SET_DISABLED": return context.disabled === event.disabled ? context : {
					...context,
					disabled: event.disabled,
					open: event.disabled ? false : context.open
				};
				default: return context;
			}
		}
	}, scope);
}
//#endregion
//#region src/core/combobox/combobox.anatomy.ts
const anatomy = createAnatomy("combobox", [
	"root",
	"label",
	"control",
	"input",
	"trigger",
	"clearTrigger",
	"listbox",
	"option"
]);
//#endregion
//#region src/core/combobox/combobox.connect.ts
const parts = anatomy.build();
/**
* Pure projection of machine state -> prop-getters, carrying the WAI-ARIA combobox (listbox-popup)
* wiring: `role="combobox"` + `aria-expanded` / `aria-controls` / `aria-activedescendant` on the input,
* `role="listbox"` + `role="option"` + `aria-selected` on the popup, plus the full keyboard contract
* (Arrow/Home/End move the highlight, Enter selects, Escape closes). `collection` is the currently
* visible (already-filtered) items, so navigation and the active-descendant id stay in sync with what
* the user sees. The framework adapter supplies `normalize` and re-invokes this on every state change.
*/
function connect(service, normalize, collection) {
	const { open, value, inputValue, highlightedValue, disabled } = service.getState();
	const { scope } = service;
	const labelId = scope.getId("label");
	const inputId = scope.getId("input");
	const listboxId = scope.getId("listbox");
	const optionId = (index) => scope.getId(`option-${index}`);
	const enabled = collection.filter((item) => !item.disabled);
	const highlightedIndex = highlightedValue === null ? -1 : collection.findIndex((i) => i.value === highlightedValue);
	const enabledCursor = highlightedValue === null ? -1 : enabled.findIndex((i) => i.value === highlightedValue);
	const send = service.send;
	function highlightAt(cursor) {
		if (enabled.length === 0) return;
		send({
			type: "HIGHLIGHT",
			value: enabled[(cursor + enabled.length) % enabled.length].value
		});
	}
	function selectItem(item) {
		if (item.disabled) return;
		send({
			type: "SELECT",
			value: item.value,
			label: item.label
		});
	}
	return {
		open,
		value,
		inputValue,
		highlightedValue,
		setOpen(next) {
			send({ type: next ? "OPEN" : "CLOSE" });
		},
		setInputValue(next) {
			send({
				type: "SET_INPUT",
				value: next
			});
		},
		select(item) {
			selectItem(item);
		},
		clear() {
			send({ type: "CLEAR" });
		},
		getRootProps() {
			return normalize.element({
				...parts.root.attrs,
				"data-state": open ? "open" : "closed"
			});
		},
		getLabelProps() {
			return normalize.element({
				...parts.label.attrs,
				id: labelId,
				for: inputId
			});
		},
		getControlProps() {
			return normalize.element({
				...parts.control.attrs,
				"data-state": open ? "open" : "closed",
				"data-disabled": disabled ? "" : void 0
			});
		},
		getInputProps() {
			return normalize.element({
				...parts.input.attrs,
				id: inputId,
				role: "combobox",
				autocomplete: "off",
				"aria-autocomplete": "list",
				"aria-expanded": open,
				"aria-controls": listboxId,
				"aria-activedescendant": open && highlightedIndex >= 0 ? optionId(highlightedIndex) : void 0,
				disabled: disabled || void 0,
				value: inputValue,
				onInput(event) {
					send({
						type: "SET_INPUT",
						value: event.target.value
					});
				},
				onKeydown(event) {
					switch (event.key) {
						case "ArrowDown":
							event.preventDefault();
							if (!open) send({ type: "OPEN" });
							highlightAt(open ? enabledCursor + 1 : 0);
							break;
						case "ArrowUp":
							event.preventDefault();
							if (!open) send({ type: "OPEN" });
							highlightAt(open ? enabledCursor - 1 : enabled.length - 1);
							break;
						case "Enter":
							if (open && highlightedValue !== null) {
								event.preventDefault();
								const item = collection.find((i) => i.value === highlightedValue);
								if (item) selectItem(item);
							}
							break;
						case "Escape":
							if (open) {
								event.preventDefault();
								send({ type: "CLOSE" });
							}
							break;
						case "Home":
							if (open) {
								event.preventDefault();
								highlightAt(0);
							}
							break;
						case "End":
							if (open) {
								event.preventDefault();
								highlightAt(enabled.length - 1);
							}
							break;
					}
				}
			});
		},
		getTriggerProps() {
			return normalize.button({
				...parts.trigger.attrs,
				type: "button",
				tabindex: -1,
				"aria-label": open ? "Close suggestions" : "Open suggestions",
				"aria-controls": listboxId,
				"aria-expanded": open,
				disabled: disabled || void 0,
				onClick() {
					send({ type: open ? "CLOSE" : "OPEN" });
				}
			});
		},
		getClearTriggerProps() {
			return normalize.button({
				...parts.clearTrigger.attrs,
				type: "button",
				tabindex: -1,
				"aria-label": "Clear selection",
				disabled: disabled || void 0,
				onClick() {
					send({ type: "CLEAR" });
				}
			});
		},
		getListboxProps() {
			return normalize.element({
				...parts.listbox.attrs,
				id: listboxId,
				role: "listbox",
				"aria-labelledby": labelId,
				hidden: !open
			});
		},
		getOptionProps(item, index) {
			const selected = item.value === value;
			const highlighted = item.value === highlightedValue;
			return normalize.element({
				...parts.option.attrs,
				id: optionId(index),
				role: "option",
				"aria-selected": selected,
				"aria-disabled": item.disabled || void 0,
				"data-highlighted": highlighted ? "" : void 0,
				"data-state": selected ? "checked" : "unchecked",
				onClick() {
					selectItem(item);
				},
				onPointermove() {
					if (!item.disabled && highlightedValue !== item.value) send({
						type: "HIGHLIGHT",
						value: item.value
					});
				}
			});
		},
		getOptionState(item) {
			return {
				highlighted: item.value === highlightedValue,
				selected: item.value === value
			};
		}
	};
}
//#endregion
//#region src/core/combobox/index.ts
var combobox_exports = /* @__PURE__ */ __exportAll({
	anatomy: () => anatomy,
	connect: () => connect,
	machine: () => machine
});
//#endregion
export { connect$1 as a, createScope as c, createNormalizer as d, disclosure_exports as i, mergeProps as l, connect as n, machine$1 as o, machine as r, createMachine as s, combobox_exports as t, createAnatomy as u };

//# sourceMappingURL=core-j3tsCnai.js.map