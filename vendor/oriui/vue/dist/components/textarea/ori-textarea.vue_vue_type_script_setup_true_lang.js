import { useOriField as e } from "../field/context.js";
import { computed as t, createCommentVNode as n, createElementBlock as r, createElementVNode as i, createTextVNode as a, defineComponent as o, mergeModels as s, mergeProps as c, normalizeClass as l, openBlock as u, toDisplayString as d, unref as f, useId as p, useModel as m, vModelText as h, withDirectives as g } from "vue";
//#region src/components/textarea/ori-textarea.vue?vue&type=script&setup=true&lang.ts
var _ = ["for"], v = {
	key: 0,
	class: "ori-textarea__required",
	"aria-hidden": "true"
}, y = [
	"id",
	"rows",
	"disabled",
	"required",
	"placeholder",
	"aria-invalid",
	"aria-describedby"
], b = ["id"], x = ["id"], S = /*@__PURE__*/ o({
	inheritAttrs: !1,
	__name: "ori-textarea",
	props: /*@__PURE__*/ s({
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
		placeholder: {},
		radius: { default: "md" },
		required: {
			type: Boolean,
			default: !1
		},
		rows: { default: 3 },
		size: { default: "md" },
		variant: { default: "outline" }
	}, {
		modelValue: {},
		modelModifiers: {}
	}),
	emits: ["update:modelValue"],
	setup(o) {
		let s = m(o, "modelValue"), S = e(), C = !!S, w = p(), T = t(() => S?.id.value ?? o.id ?? w), E = t(() => `${T.value}-hint`), D = t(() => `${T.value}-error`), O = t(() => S ? S.invalid.value : o.invalid || !!o.error), k = t(() => o.required || (S?.required.value ?? !1)), A = t(() => o.disabled || (S?.disabled.value ?? !1)), j = t(() => S?.size.value ?? o.size), M = t(() => {
			if (S) return S.describedBy.value;
			let e = [o.error ? D.value : o.hint ? E.value : "", o.describedby].filter(Boolean);
			return e.length ? e.join(" ") : void 0;
		});
		return (e, t) => (u(), r("div", { class: l([
			"ori-textarea",
			`ori-color_${o.color}`,
			`ori-font-size_${j.value}`,
			`ori-textarea_${o.variant}`,
			`ori-textarea_${j.value}`,
			{ "ori-textarea_fluid": o.fluid || f(C) }
		]) }, [
			o.label && !f(C) ? (u(), r("label", {
				key: 0,
				for: T.value,
				class: "ori-textarea__label"
			}, [a(d(o.label), 1), o.required ? (u(), r("span", v, "*")) : n("", !0)], 8, _)) : n("", !0),
			g(i("textarea", c(e.$attrs, {
				id: T.value,
				"onUpdate:modelValue": t[0] ||= (e) => s.value = e,
				class: ["ori-textarea__field", `ori-size-radius_${o.radius}`],
				rows: o.rows,
				disabled: A.value,
				required: k.value,
				placeholder: o.placeholder,
				"aria-invalid": O.value ? "true" : void 0,
				"aria-describedby": M.value
			}), null, 16, y), [[h, s.value]]),
			o.error && !f(C) ? (u(), r("p", {
				key: 1,
				id: D.value,
				class: "ori-textarea__error",
				role: "alert"
			}, d(o.error), 9, b)) : o.hint && !f(C) ? (u(), r("p", {
				key: 2,
				id: E.value,
				class: "ori-textarea__hint"
			}, d(o.hint), 9, x)) : n("", !0)
		], 2));
	}
});
//#endregion
export { S as default };

