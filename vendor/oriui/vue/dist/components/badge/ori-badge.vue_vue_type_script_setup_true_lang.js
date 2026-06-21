import { Fragment as e, computed as t, createCommentVNode as n, createElementBlock as r, createElementVNode as i, createTextVNode as a, defineComponent as o, mergeProps as s, openBlock as c, renderSlot as l, toDisplayString as u } from "vue";
//#region src/components/badge/ori-badge.vue?vue&type=script&setup=true&lang.ts
var d = {
	key: 0,
	class: "ori-badge-anchor"
}, f = ["aria-label", "aria-hidden"], p = ["aria-label", "aria-hidden"], m = /*@__PURE__*/ o({
	inheritAttrs: !1,
	__name: "ori-badge",
	props: {
		color: { default: "primary" },
		content: {},
		dot: {
			type: Boolean,
			default: !1
		},
		floating: { type: Boolean },
		label: {},
		max: {},
		radius: { default: "rounded" },
		variant: { default: "fill" }
	},
	setup(o) {
		let m = t(() => typeof o.content == "number" && typeof o.max == "number" && o.content > o.max ? `${o.max}+` : o.content), h = t(() => !o.label && (o.dot || m.value === void 0 || m.value === ""));
		return (t, g) => t.$slots.default ? (c(), r("span", d, [l(t.$slots, "default"), i("span", s({ class: ["ori-badge", {
			"ori-badge_dot": o.dot,
			"ori-badge_floating": o.floating,
			[`ori-size-radius_${o.radius}`]: o.radius,
			[`ori-variant_${o.variant}`]: o.variant,
			[`ori-color_${o.color}`]: o.color
		}] }, t.$attrs, {
			"aria-label": o.label || void 0,
			"aria-hidden": h.value ? "true" : void 0
		}), [o.dot ? n("", !0) : (c(), r(e, { key: 0 }, [a(u(m.value), 1)], 64))], 16, f)])) : (c(), r("span", s({
			key: 1,
			class: ["ori-badge", {
				"ori-badge_dot": o.dot,
				[`ori-size-radius_${o.radius}`]: o.radius,
				[`ori-variant_${o.variant}`]: o.variant,
				[`ori-color_${o.color}`]: o.color
			}]
		}, t.$attrs, {
			"aria-label": o.label || void 0,
			"aria-hidden": h.value ? "true" : void 0
		}), [o.dot ? n("", !0) : (c(), r(e, { key: 0 }, [a(u(m.value), 1)], 64))], 16, p));
	}
});
//#endregion
export { m as default };

