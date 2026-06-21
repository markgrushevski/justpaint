import e from "../icon/ori-icon.js";
import { createBlock as t, createCommentVNode as n, createElementBlock as r, createElementVNode as i, createTextVNode as a, defineComponent as o, normalizeClass as s, openBlock as c, renderSlot as l, toDisplayString as u, unref as d } from "vue";
//#region src/components/toast/ori-toast.vue?vue&type=script&setup=true&lang.ts
var f = ["role"], p = { class: "ori-toast__body" }, m = {
	key: 0,
	class: "ori-toast__title"
}, h = {
	key: 1,
	class: "ori-toast__text"
}, g = /*@__PURE__*/ o({
	__name: "ori-toast",
	props: {
		closable: {
			type: Boolean,
			default: !1
		},
		color: { default: "surface" },
		icon: {},
		text: {},
		title: {}
	},
	emits: ["close"],
	setup(o) {
		return (g, _) => (c(), r("div", {
			class: s(["ori-toast", { [`ori-color_${o.color}`]: o.color }]),
			role: o.color === "danger" ? "alert" : "status"
		}, [
			o.icon ? (c(), t(d(e), {
				key: 0,
				icon: o.icon,
				class: "ori-toast__icon"
			}, null, 8, ["icon"])) : n("", !0),
			i("div", p, [o.title ? (c(), r("div", m, u(o.title), 1)) : n("", !0), o.text || g.$slots.default ? (c(), r("div", h, [l(g.$slots, "default", {}, () => [a(u(o.text), 1)])])) : n("", !0)]),
			o.closable ? (c(), r("button", {
				key: 1,
				class: "ori-toast__close",
				type: "button",
				"aria-label": "Dismiss notification",
				onClick: _[0] ||= (e) => g.$emit("close")
			}, " × ")) : n("", !0)
		], 10, f));
	}
});
//#endregion
export { g as default };

