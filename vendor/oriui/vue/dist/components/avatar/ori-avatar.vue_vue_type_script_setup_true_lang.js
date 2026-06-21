import { computed as e, createCommentVNode as t, createElementBlock as n, createElementVNode as r, defineComponent as i, mergeProps as a, normalizeClass as o, openBlock as s, ref as c, toDisplayString as l, vShow as u, withDirectives as d } from "vue";
//#region src/components/avatar/ori-avatar.vue?vue&type=script&setup=true&lang.ts
var f = ["alt"], p = {
	key: 1,
	"aria-hidden": "true",
	class: "ori-avatar__backdrop"
}, m = {
	key: 2,
	class: "ori-avatar__text"
}, h = { class: "ori-avatar__title" }, g = { class: "ori-avatar__subtitle" }, _ = /*@__PURE__*/ i({
	inheritAttrs: !1,
	__name: "ori-avatar",
	props: {
		color: {},
		inline: { type: Boolean },
		radius: { default: "rounded" },
		size: { default: "lg" },
		spaced: { type: Boolean },
		subtitle: {},
		text: {},
		title: {},
		reverse: { type: Boolean }
	},
	setup(i) {
		let _ = c(!1), v = e(() => {
			let e = i.text?.trim()?.split(" ") ?? [];
			if (e.length > 0) {
				let [t, n] = e, r = t?.[0] ?? "", i = n?.[0] ?? "";
				return r.toUpperCase() + i.toUpperCase();
			}
			return "";
		});
		return (e, c) => (s(), n("div", { class: o(["ori-avatar", {
			"ori-avatar_reverse": i.reverse,
			"ori-avatar_inline": i.inline,
			"ori-avatar_titled": i.title || i.subtitle,
			[`ori-avatar_${i.size}`]: i.size,
			[`ori-size-action-space_${i.size}`]: i.size && i.spaced,
			[`ori-size-radius_${i.radius}`]: i.radius,
			[`ori-font-size_${i.size}`]: i.size,
			[`ori-color_${i.color}`]: i.color
		}]) }, [
			e.$attrs.src ? d((s(), n("img", a({
				key: 0,
				class: "ori-avatar__image"
			}, e.$attrs, {
				alt: i.text || "",
				onLoad: c[0] ||= (e) => _.value = !0
			}), null, 16, f)), [[u, _.value]]) : t("", !0),
			!e.$attrs.src || !_.value ? (s(), n("div", p, l(v.value), 1)) : t("", !0),
			i.title || i.subtitle ? (s(), n("div", m, [r("div", h, l(i.title), 1), r("div", g, l(i.subtitle), 1)])) : t("", !0)
		], 2));
	}
});
//#endregion
export { _ as default };

