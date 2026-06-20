import { computed as e, createCommentVNode as t, createElementBlock as n, createElementVNode as r, defineComponent as i, mergeModels as a, mergeProps as o, normalizeClass as s, openBlock as c, toDisplayString as l, useId as u, useModel as d, vModelCheckbox as f, withDirectives as p } from "vue";
//#region src/components/checkbox/ori-checkbox.vue?vue&type=script&setup=true&lang.ts
var m = ["for"], h = [
	"id",
	"value",
	"disabled",
	"required",
	"aria-invalid"
], g = {
	key: 0,
	class: "ori-checkbox__label"
}, _ = /*@__PURE__*/ i({
	inheritAttrs: !1,
	__name: "ori-checkbox",
	props: /*@__PURE__*/ a({
		color: { default: "primary" },
		disabled: {
			type: Boolean,
			default: !1
		},
		id: {},
		invalid: { type: Boolean },
		label: {},
		required: { type: Boolean },
		size: { default: "md" },
		value: {}
	}, {
		modelValue: { type: [Boolean, Array] },
		modelModifiers: {}
	}),
	emits: ["update:modelValue"],
	setup(i) {
		let a = d(i, "modelValue"), _ = u(), v = e(() => i.id ?? _);
		return (e, u) => (c(), n("label", {
			for: v.value,
			class: s([
				"ori-checkbox",
				`ori-color_${i.color}`,
				`ori-font-size_${i.size}`,
				{ "ori-checkbox_disabled": i.disabled }
			])
		}, [
			p(r("input", o(e.$attrs, {
				id: v.value,
				"onUpdate:modelValue": u[0] ||= (e) => a.value = e,
				class: "ori-checkbox__input",
				type: "checkbox",
				value: i.value,
				disabled: i.disabled,
				required: i.required,
				"aria-invalid": i.invalid ? "true" : void 0
			}), null, 16, h), [[f, a.value]]),
			u[1] ||= r("span", {
				class: "ori-checkbox__box",
				"aria-hidden": "true"
			}, null, -1),
			i.label ? (c(), n("span", g, l(i.label), 1)) : t("", !0)
		], 10, m));
	}
});
//#endregion
export { _ as default };

//# sourceMappingURL=ori-checkbox.vue_vue_type_script_setup_true_lang.js.map