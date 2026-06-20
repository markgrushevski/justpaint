import e from "../spinner/ori-spinner.js";
import t from "../icon/ori-icon.js";
import { createBlock as n, createCommentVNode as r, createElementBlock as i, defineComponent as a, normalizeClass as o, openBlock as s, renderSlot as c, resolveDynamicComponent as l, toDisplayString as u, unref as d, withCtx as f } from "vue";
//#region src/components/button/ori-button.vue?vue&type=script&setup=true&lang.ts
var p = {
	key: 2,
	class: "ori-button__text"
}, m = /*@__PURE__*/ a({
	__name: "ori-button",
	props: {
		active: { type: Boolean },
		as: { default: "button" },
		color: { default: "primary" },
		disabled: { type: Boolean },
		fluid: { type: Boolean },
		icon: {},
		iconPosition: { default: "left" },
		loading: { type: Boolean },
		radius: { default: "rounded" },
		size: { default: "md" },
		text: {},
		variant: { default: "fill" }
	},
	setup(a) {
		return (m, h) => (s(), n(l(a.as), {
			class: o(["ori-button", {
				"ori-button_icon": !a.text,
				"ori-button_fluid": a.fluid,
				[`ori-button_icon-position_${a.iconPosition}`]: a.iconPosition,
				[`ori-button_${a.size}`]: a.size,
				[`ori-size-radius_${a.radius}`]: a.radius,
				[`ori-font-size_${a.size}`]: a.size,
				[`ori-variant_${a.variant}`]: a.variant,
				[`ori-color_${a.color}`]: a.color
			}]),
			type: a.as === "button" ? "button" : void 0,
			disabled: a.as === "button" && (a.disabled || a.loading) ? !0 : void 0,
			"aria-disabled": a.disabled ? "true" : void 0,
			"aria-busy": a.loading ? "true" : void 0,
			"data-active": a.active ? "" : void 0,
			tabindex: a.disabled && a.as !== "button" ? -1 : void 0
		}, {
			default: f(() => [c(m.$slots, "default", {}, () => [a.icon && !a.loading ? (s(), n(d(t), {
				key: 0,
				icon: a.icon,
				class: "ori-button__icon"
			}, null, 8, ["icon"])) : a.loading ? (s(), n(d(e), {
				key: 1,
				"aria-hidden": "true",
				class: "ori-button__icon"
			})) : r("", !0), a.text ? (s(), i("span", p, u(a.text), 1)) : r("", !0)])]),
			_: 3
		}, 8, [
			"class",
			"type",
			"disabled",
			"aria-disabled",
			"aria-busy",
			"data-active",
			"tabindex"
		]));
	}
});
//#endregion
export { m as default };

//# sourceMappingURL=ori-button.vue_vue_type_script_setup_true_lang.js.map