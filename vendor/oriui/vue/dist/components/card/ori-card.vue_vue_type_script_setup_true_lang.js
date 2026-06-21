import e from "../icon/ori-icon.js";
import t from "../avatar/ori-avatar.js";
import { createBlock as n, createCommentVNode as r, createElementBlock as i, createElementVNode as a, createTextVNode as o, defineComponent as s, normalizeClass as c, openBlock as l, renderSlot as u, toDisplayString as d, unref as f } from "vue";
//#region src/components/card/ori-card.vue?vue&type=script&setup=true&lang.ts
var p = ["aria-disabled", "aria-busy"], m = { class: "ori-card__header" }, h = { class: "ori-card__header-prepend" }, g = { class: "ori-card__headline" }, _ = { class: "ori-card__title" }, v = { class: "ori-card__subtitle" }, y = { class: "ori-card__header-append" }, b = {
	key: 1,
	class: "ori-card__body"
}, x = /*@__PURE__*/ s({
	__name: "ori-card",
	props: {
		appendAvatar: {},
		appendIcon: {},
		color: { default: "surface" },
		disabled: { type: Boolean },
		fluid: { type: Boolean },
		image: {},
		loading: { type: Boolean },
		prependAvatar: {},
		prependIcon: {},
		radius: { default: "lg" },
		reverseAppendedActions: { type: Boolean },
		reversePrependedActions: { type: Boolean },
		subtitle: {},
		text: {},
		title: {},
		variant: { default: "fill" },
		row: { type: Boolean }
	},
	setup(s) {
		return (x, S) => (l(), i("div", {
			class: c(["ori-card", {
				"ori-card_icon": !s.text,
				"ori-card_fluid": s.fluid,
				"ori-card_row": s.row,
				[`ori-size-radius_${s.radius}`]: s.radius,
				[`ori-variant_${s.variant}`]: s.variant,
				[`ori-color_${s.color}`]: s.color
			}]),
			"aria-disabled": s.disabled ? "true" : void 0,
			"aria-busy": s.loading ? "true" : void 0
		}, [u(x.$slots, "default", {}, () => [
			x.$slots["actions-prepend"] ? (l(), i("div", {
				key: 0,
				class: c([{ "ori-card__actions_reverse": s.reversePrependedActions }, "ori-card__actions"])
			}, [u(x.$slots, "actions-prepend")], 2)) : r("", !0),
			a("div", m, [
				a("div", h, [u(x.$slots, "header-prepend", {}, () => [s.prependAvatar ? (l(), n(f(t), {
					key: 0,
					src: s.prependAvatar
				}, null, 8, ["src"])) : r("", !0), s.prependIcon ? (l(), n(f(e), {
					key: 1,
					icon: s.prependIcon,
					size: "sm"
				}, null, 8, ["icon"])) : r("", !0)])]),
				a("div", g, [a("div", _, [u(x.$slots, "title", {}, () => [o(d(s.title), 1)])]), a("div", v, [u(x.$slots, "subtitle", {}, () => [o(d(s.subtitle), 1)])])]),
				a("div", y, [u(x.$slots, "header-append", {}, () => [s.appendAvatar ? (l(), n(f(t), {
					key: 0,
					src: s.appendAvatar
				}, null, 8, ["src"])) : r("", !0), s.appendIcon ? (l(), n(f(e), {
					key: 1,
					icon: s.appendIcon,
					size: "sm"
				}, null, 8, ["icon"])) : r("", !0)])])
			]),
			x.$slots.body || s.text ? (l(), i("div", b, [u(x.$slots, "body", {}, () => [o(d(s.text), 1)])])) : r("", !0),
			x.$slots["actions-append"] ? (l(), i("div", {
				key: 2,
				class: c([{ "ori-card__actions_reverse": s.reverseAppendedActions }, "ori-card__actions"])
			}, [u(x.$slots, "actions-append")], 2)) : r("", !0)
		])], 10, p));
	}
});
//#endregion
export { x as default };

