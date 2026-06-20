import { Fragment as e, computed as t, createCommentVNode as n, createElementBlock as r, createElementVNode as i, defineComponent as a, mergeModels as o, normalizeClass as s, openBlock as c, renderList as l, toDisplayString as u, useId as d, useModel as f, vModelRadio as p, withDirectives as m } from "vue";
//#region src/components/radio/ori-radio-group.vue?vue&type=script&setup=true&lang.ts
var h = ["aria-labelledby", "aria-required"], g = ["id"], _ = { class: "ori-radio-group__options" }, v = [
	"name",
	"value",
	"disabled",
	"required"
], y = { class: "ori-radio__label" }, b = /*@__PURE__*/ a({
	__name: "ori-radio-group",
	props: /*@__PURE__*/ o({
		color: { default: "primary" },
		disabled: {
			type: Boolean,
			default: !1
		},
		inline: { type: Boolean },
		label: {},
		name: {},
		options: { default: () => [] },
		required: { type: Boolean },
		size: { default: "md" }
	}, {
		modelValue: {},
		modelModifiers: {}
	}),
	emits: ["update:modelValue"],
	setup(a) {
		let o = f(a, "modelValue"), b = d(), x = t(() => a.name ?? b), S = t(() => `${b}-label`);
		return (t, d) => (c(), r("div", {
			class: s([
				"ori-radio-group",
				`ori-color_${a.color}`,
				`ori-font-size_${a.size}`,
				{ "ori-radio-group_inline": a.inline }
			]),
			role: "radiogroup",
			"aria-labelledby": a.label ? S.value : void 0,
			"aria-required": a.required ? "true" : void 0
		}, [a.label ? (c(), r("div", {
			key: 0,
			id: S.value,
			class: "ori-radio-group__label"
		}, u(a.label), 9, g)) : n("", !0), i("div", _, [(c(!0), r(e, null, l(a.options, (e) => (c(), r("label", {
			key: e.value,
			class: s(["ori-radio", { "ori-radio_disabled": a.disabled || e.disabled }])
		}, [
			m(i("input", {
				"onUpdate:modelValue": d[0] ||= (e) => o.value = e,
				class: "ori-radio__input",
				type: "radio",
				name: x.value,
				value: e.value,
				disabled: a.disabled || e.disabled,
				required: a.required
			}, null, 8, v), [[p, o.value]]),
			d[1] ||= i("span", {
				class: "ori-radio__circle",
				"aria-hidden": "true"
			}, null, -1),
			i("span", y, u(e.label), 1)
		], 2))), 128))])], 10, h));
	}
});
//#endregion
export { b as default };

//# sourceMappingURL=ori-radio-group.vue_vue_type_script_setup_true_lang.js.map