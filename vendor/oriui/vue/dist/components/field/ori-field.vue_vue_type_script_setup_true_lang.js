import { oriFieldKey as e } from "./context.js";
import { computed as t, createCommentVNode as n, createElementBlock as r, createTextVNode as i, defineComponent as a, guardReactiveProps as o, normalizeClass as s, normalizeProps as c, openBlock as l, provide as u, renderSlot as d, toDisplayString as f, useId as p } from "vue";
//#region src/components/field/ori-field.vue?vue&type=script&setup=true&lang.ts
var m = ["for"], h = {
	key: 0,
	class: "ori-field__required",
	"aria-hidden": "true"
}, g = ["id"], _ = ["id"], v = /*@__PURE__*/ a({
	__name: "ori-field",
	props: {
		describedby: {},
		disabled: {
			type: Boolean,
			default: !1
		},
		error: {},
		fluid: { type: Boolean },
		hint: {},
		id: {},
		invalid: {
			type: Boolean,
			default: !1
		},
		label: {},
		required: {
			type: Boolean,
			default: !1
		},
		size: { default: "md" }
	},
	setup(a) {
		let v = p(), y = t(() => a.id ?? v), b = t(() => `${y.value}-hint`), x = t(() => `${y.value}-error`), S = t(() => a.invalid || !!a.error), C = t(() => {
			let e = [a.error ? x.value : a.hint ? b.value : "", a.describedby].filter(Boolean);
			return e.length ? e.join(" ") : void 0;
		});
		u(e, {
			id: y,
			describedBy: C,
			invalid: S,
			required: t(() => a.required),
			disabled: t(() => a.disabled),
			size: t(() => a.size)
		});
		let w = t(() => ({
			id: y.value,
			"aria-describedby": C.value,
			"aria-invalid": S.value ? "true" : void 0,
			disabled: a.disabled || void 0,
			required: a.required || void 0
		})), T = t(() => ({
			id: y.value,
			invalid: S.value,
			describedby: C.value,
			controlAttrs: w.value
		}));
		return (e, t) => (l(), r("div", { class: s([
			"ori-field",
			`ori-font-size_${a.size}`,
			{ "ori-field_fluid": a.fluid }
		]) }, [
			a.label ? (l(), r("label", {
				key: 0,
				for: y.value,
				class: "ori-field__label"
			}, [i(f(a.label), 1), a.required ? (l(), r("span", h, "*")) : n("", !0)], 8, m)) : n("", !0),
			d(e.$slots, "default", c(o(T.value))),
			a.error ? (l(), r("p", {
				key: 1,
				id: x.value,
				class: "ori-field__error",
				role: "alert"
			}, f(a.error), 9, g)) : a.hint ? (l(), r("p", {
				key: 2,
				id: b.value,
				class: "ori-field__hint"
			}, f(a.hint), 9, _)) : n("", !0)
		], 2));
	}
});
//#endregion
export { v as default };

