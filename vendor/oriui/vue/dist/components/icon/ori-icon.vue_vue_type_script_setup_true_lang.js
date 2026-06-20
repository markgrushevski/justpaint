import { createCommentVNode as e, createElementBlock as t, createElementVNode as n, defineComponent as r, normalizeClass as i, openBlock as a, renderSlot as o } from "vue";
//#region src/components/icon/ori-icon.vue?vue&type=script&setup=true&lang.ts
var s = [
	"role",
	"aria-label",
	"aria-hidden"
], c = {
	key: 0,
	viewBox: "0 0 24 24",
	xmlns: "http://www.w3.org/2000/svg"
}, l = ["d"], u = /*@__PURE__*/ r({
	__name: "ori-icon",
	props: {
		color: {},
		icon: {},
		inline: { type: Boolean },
		label: {},
		size: { default: "text" },
		spaced: { type: Boolean }
	},
	setup(r) {
		return (u, d) => (a(), t("i", {
			class: i(["ori-icon", {
				"ori-icon_inline": r.inline,
				[`ori-icon_${r.size}`]: r.size,
				[`ori-size-action-space_${r.size}`]: r.size && r.spaced,
				[`ori-color_${r.color}`]: r.color
			}]),
			role: r.label ? "img" : void 0,
			"aria-label": r.label || void 0,
			"aria-hidden": r.label ? void 0 : "true"
		}, [o(u.$slots, "default", {}, () => [r.icon ? (a(), t("svg", c, [n("path", { d: r.icon }, null, 8, l)])) : e("", !0)])], 10, s));
	}
});
//#endregion
export { u as default };

//# sourceMappingURL=ori-icon.vue_vue_type_script_setup_true_lang.js.map