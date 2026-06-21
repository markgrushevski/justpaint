import { computed as e, createCommentVNode as t, createElementBlock as n, createElementVNode as r, defineComponent as i, mergeModels as a, mergeProps as o, normalizeClass as s, openBlock as c, toDisplayString as l, useId as u, useModel as d, vModelCheckbox as f, withDirectives as p } from "vue";
//#region src/components/switch/ori-switch.vue?vue&type=script&setup=true&lang.ts
var m = ["for"], h = [
	"id",
	"disabled",
	"required",
	"aria-invalid"
], g = {
	key: 0,
	class: "ori-switch__label"
}, _ = /*@__PURE__*/ i({
	inheritAttrs: !1,
	__name: "ori-switch",
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
		size: { default: "md" }
	}, {
		modelValue: { type: Boolean },
		modelModifiers: {}
	}),
	emits: ["update:modelValue"],
	setup(i) {
		let a = d(i, "modelValue"), _ = u(), v = e(() => i.id ?? _);
		return (e, u) => (c(), n("label", {
			for: v.value,
			class: s([
				"ori-switch",
				`ori-color_${i.color}`,
				`ori-font-size_${i.size}`,
				{ "ori-switch_disabled": i.disabled }
			])
		}, [
			p(r("input", o(e.$attrs, {
				id: v.value,
				"onUpdate:modelValue": u[0] ||= (e) => a.value = e,
				class: "ori-switch__input",
				type: "checkbox",
				role: "switch",
				disabled: i.disabled,
				required: i.required,
				"aria-invalid": i.invalid ? "true" : void 0
			}), null, 16, h), [[f, a.value]]),
			u[1] ||= r("span", {
				class: "ori-switch__track",
				"aria-hidden": "true"
			}, [r("span", { class: "ori-switch__thumb" })], -1),
			i.label ? (c(), n("span", g, l(i.label), 1)) : t("", !0)
		], 10, m));
	}
});
//#endregion
export { _ as default };

