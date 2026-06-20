import e from "../icon/ori-icon.js";
import { computed as t, createBlock as n, createCommentVNode as r, createElementBlock as i, createElementVNode as a, createTextVNode as o, createVNode as s, defineComponent as c, normalizeClass as l, openBlock as u, renderSlot as d, toDisplayString as f, unref as p } from "vue";
//#region src/components/alert/ori-alert.vue?vue&type=script&setup=true&lang.ts
var m = ["role"], h = {
	key: 0,
	class: "ori-alert__icon"
}, g = { class: "ori-alert__content" }, _ = {
	key: 0,
	class: "ori-alert__title"
}, v = {
	key: 1,
	class: "ori-alert__body"
}, y = {
	key: 2,
	class: "ori-alert__actions"
}, b = ["aria-label"], x = "M6.4 19 5 17.6l5.6-5.6L5 6.4 6.4 5l5.6 5.6L17.6 5 19 6.4 13.4 12l5.6 5.6-1.4 1.4-5.6-5.6Z", S = /*@__PURE__*/ c({
	__name: "ori-alert",
	props: {
		closable: { type: Boolean },
		closeLabel: { default: "Dismiss" },
		color: { default: "info" },
		icon: {},
		live: {},
		radius: { default: "md" },
		size: { default: "md" },
		text: {},
		title: {},
		variant: { default: "tonal" }
	},
	emits: ["close"],
	setup(c, { emit: S }) {
		let C = S, w = t(() => c.live ?? (c.color === "danger" || c.color === "warn" ? "assertive" : "polite")), T = t(() => {
			if (w.value === "assertive") return "alert";
			if (w.value === "polite") return "status";
		});
		return (t, S) => (u(), i("div", {
			class: l(["ori-alert", {
				[`ori-size-radius_${c.radius}`]: c.radius,
				[`ori-font-size_${c.size}`]: c.size,
				[`ori-variant_${c.variant}`]: c.variant,
				[`ori-color_${c.color}`]: c.color
			}]),
			role: T.value
		}, [
			c.icon || t.$slots.icon ? (u(), i("div", h, [d(t.$slots, "icon", {}, () => [c.icon ? (u(), n(p(e), {
				key: 0,
				icon: c.icon,
				size: "sm"
			}, null, 8, ["icon"])) : r("", !0)])])) : r("", !0),
			a("div", g, [
				c.title || t.$slots.title ? (u(), i("div", _, [d(t.$slots, "title", {}, () => [o(f(c.title), 1)])])) : r("", !0),
				c.text || t.$slots.default ? (u(), i("div", v, [d(t.$slots, "default", {}, () => [o(f(c.text), 1)])])) : r("", !0),
				t.$slots.actions ? (u(), i("div", y, [d(t.$slots, "actions")])) : r("", !0)
			]),
			c.closable ? (u(), i("button", {
				key: 1,
				type: "button",
				class: "ori-alert__close",
				"aria-label": c.closeLabel,
				onClick: S[0] ||= (e) => C("close")
			}, [s(p(e), {
				icon: x,
				size: "sm"
			})], 8, b)) : r("", !0)
		], 10, m));
	}
});
//#endregion
export { S as default };

//# sourceMappingURL=ori-alert.vue_vue_type_script_setup_true_lang.js.map