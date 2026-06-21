import { useOriField as e } from "../field/context.js";
import { Fragment as t, computed as n, createCommentVNode as r, createElementBlock as i, createElementVNode as a, createTextVNode as o, defineComponent as s, mergeModels as c, mergeProps as l, normalizeClass as u, openBlock as d, renderList as f, renderSlot as p, toDisplayString as m, unref as h, useId as g, useModel as _, vModelSelect as v, withDirectives as y } from "vue";
//#region src/components/select/ori-select.vue?vue&type=script&setup=true&lang.ts
var b = ["for"], x = {
	key: 0,
	class: "ori-select__required",
	"aria-hidden": "true"
}, S = { class: "ori-select__control-wrap" }, C = [
	"id",
	"disabled",
	"required",
	"aria-invalid",
	"aria-describedby"
], w = {
	key: 0,
	value: "",
	disabled: ""
}, T = ["value", "disabled"], E = ["id"], D = ["id"], O = /*@__PURE__*/ s({
	inheritAttrs: !1,
	__name: "ori-select",
	props: /*@__PURE__*/ c({
		color: { default: "primary" },
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
		options: { default: () => [] },
		placeholder: {},
		radius: { default: "md" },
		required: {
			type: Boolean,
			default: !1
		},
		size: { default: "md" }
	}, {
		modelValue: {},
		modelModifiers: {}
	}),
	emits: ["update:modelValue"],
	setup(s) {
		let c = _(s, "modelValue"), O = e(), k = !!O, A = g(), j = n(() => O?.id.value ?? s.id ?? A), M = n(() => `${j.value}-hint`), N = n(() => `${j.value}-error`), P = n(() => O ? O.invalid.value : s.invalid || !!s.error), F = n(() => s.required || (O?.required.value ?? !1)), I = n(() => s.disabled || (O?.disabled.value ?? !1)), L = n(() => O?.size.value ?? s.size), R = n(() => {
			if (O) return O.describedBy.value;
			let e = [s.error ? N.value : s.hint ? M.value : "", s.describedby].filter(Boolean);
			return e.length ? e.join(" ") : void 0;
		});
		return (e, n) => (d(), i("div", { class: u([
			"ori-select",
			`ori-color_${s.color}`,
			`ori-font-size_${L.value}`,
			`ori-select_${L.value}`,
			{ "ori-select_fluid": s.fluid || h(k) }
		]) }, [
			s.label && !h(k) ? (d(), i("label", {
				key: 0,
				for: j.value,
				class: "ori-select__label"
			}, [o(m(s.label), 1), s.required ? (d(), i("span", x, "*")) : r("", !0)], 8, b)) : r("", !0),
			a("div", S, [y(a("select", l(e.$attrs, {
				id: j.value,
				"onUpdate:modelValue": n[0] ||= (e) => c.value = e,
				class: ["ori-select__control", `ori-size-radius_${s.radius}`],
				disabled: I.value,
				required: F.value,
				"aria-invalid": P.value ? "true" : void 0,
				"aria-describedby": R.value
			}), [s.placeholder ? (d(), i("option", w, m(s.placeholder), 1)) : r("", !0), p(e.$slots, "default", {}, () => [(d(!0), i(t, null, f(s.options, (e) => (d(), i("option", {
				key: e.value,
				value: e.value,
				disabled: e.disabled
			}, m(e.label), 9, T))), 128))])], 16, C), [[v, c.value]]), n[1] ||= a("span", {
				class: "ori-select__chevron",
				"aria-hidden": "true"
			}, [a("svg", {
				viewBox: "0 0 24 24",
				xmlns: "http://www.w3.org/2000/svg"
			}, [a("path", {
				d: "M6 9l6 6 6-6",
				fill: "none",
				stroke: "currentcolor",
				"stroke-width": "2",
				"stroke-linecap": "round",
				"stroke-linejoin": "round"
			})])], -1)]),
			s.error && !h(k) ? (d(), i("p", {
				key: 1,
				id: N.value,
				class: "ori-select__error",
				role: "alert"
			}, m(s.error), 9, E)) : s.hint && !h(k) ? (d(), i("p", {
				key: 2,
				id: M.value,
				class: "ori-select__hint"
			}, m(s.hint), 9, D)) : r("", !0)
		], 2));
	}
});
//#endregion
export { O as default };

