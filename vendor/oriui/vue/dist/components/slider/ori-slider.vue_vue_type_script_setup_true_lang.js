import { computed as e, createCommentVNode as t, createElementBlock as n, createElementVNode as r, defineComponent as i, mergeProps as a, normalizeClass as o, openBlock as s, toDisplayString as c, unref as l, useId as u } from "vue";
//#region src/components/slider/ori-slider.vue?vue&type=script&setup=true&lang.ts
var d = ["data-disabled"], f = ["for"], p = { key: 0 }, m = {
	key: 1,
	class: "ori-slider__value"
}, h = [
	"id",
	"min",
	"max",
	"step",
	"value",
	"disabled"
], g = /*@__PURE__*/ i({
	inheritAttrs: !1,
	__name: "ori-slider",
	props: {
		color: { default: "primary" },
		disabled: { type: Boolean },
		label: {},
		max: { default: 100 },
		min: { default: 0 },
		modelValue: {},
		showValue: { type: Boolean },
		step: { default: 1 }
	},
	emits: ["update:modelValue"],
	setup(i, { emit: g }) {
		let _ = g, v = u(), y = e(() => i.modelValue ?? i.min), b = e(() => {
			let e = i.max - i.min;
			return e > 0 ? (y.value - i.min) / e * 100 : 0;
		});
		function x(e) {
			_("update:modelValue", Number(e.target.value));
		}
		return (e, u) => (s(), n("div", {
			class: o(["ori-slider", { [`ori-color_${i.color}`]: i.color }]),
			"data-disabled": i.disabled ? "" : void 0
		}, [i.label || i.showValue ? (s(), n("label", {
			key: 0,
			for: l(v),
			class: "ori-slider__label"
		}, [i.label ? (s(), n("span", p, c(i.label), 1)) : t("", !0), i.showValue ? (s(), n("span", m, c(y.value), 1)) : t("", !0)], 8, f)) : t("", !0), r("input", a({
			id: l(v),
			class: "ori-slider__input",
			type: "range"
		}, e.$attrs, {
			min: i.min,
			max: i.max,
			step: i.step,
			value: y.value,
			disabled: i.disabled,
			style: { "--ori-slider-pct": `${b.value}%` },
			onInput: x
		}), null, 16, h)], 10, d));
	}
});
//#endregion
export { g as default };

