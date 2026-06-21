import e from "../icon/ori-icon.js";
import { createBlock as t, createCommentVNode as n, createElementBlock as r, createElementVNode as i, createTextVNode as a, createVNode as o, defineComponent as s, normalizeClass as c, openBlock as l, renderSlot as u, toDisplayString as d, unref as f } from "vue";
//#region src/components/tag/ori-tag.vue?vue&type=script&setup=true&lang.ts
var p = ["aria-disabled"], m = { class: "ori-tag__text" }, h = ["aria-label", "disabled"], g = "M6.4 19 5 17.6l5.6-5.6L5 6.4 6.4 5l5.6 5.6L17.6 5 19 6.4 13.4 12l5.6 5.6-1.4 1.4-5.6-5.6Z", _ = /*@__PURE__*/ s({
	__name: "ori-tag",
	props: {
		appendIcon: {},
		closable: { type: Boolean },
		closeLabel: { default: "Remove" },
		color: { default: "primary" },
		disabled: { type: Boolean },
		prependIcon: {},
		radius: { default: "rounded" },
		size: { default: "sm" },
		text: {},
		variant: { default: "tonal" }
	},
	emits: ["close"],
	setup(s, { emit: _ }) {
		let v = _;
		return (_, y) => (l(), r("span", {
			class: c(["ori-tag", {
				[`ori-size-radius_${s.radius}`]: s.radius,
				[`ori-font-size_${s.size}`]: s.size,
				[`ori-variant_${s.variant}`]: s.variant,
				[`ori-color_${s.color}`]: s.color
			}]),
			"aria-disabled": s.disabled ? "true" : void 0
		}, [
			s.prependIcon ? (l(), t(f(e), {
				key: 0,
				icon: s.prependIcon,
				class: "ori-tag__icon"
			}, null, 8, ["icon"])) : n("", !0),
			i("span", m, [u(_.$slots, "default", {}, () => [a(d(s.text), 1)])]),
			s.appendIcon ? (l(), t(f(e), {
				key: 1,
				icon: s.appendIcon,
				class: "ori-tag__icon"
			}, null, 8, ["icon"])) : n("", !0),
			s.closable ? (l(), r("button", {
				key: 2,
				type: "button",
				class: "ori-tag__close",
				"aria-label": s.closeLabel,
				disabled: s.disabled,
				onClick: y[0] ||= (e) => v("close")
			}, [o(f(e), {
				icon: g,
				class: "ori-tag__close-icon"
			})], 8, h)) : n("", !0)
		], 10, p));
	}
});
//#endregion
export { _ as default };

