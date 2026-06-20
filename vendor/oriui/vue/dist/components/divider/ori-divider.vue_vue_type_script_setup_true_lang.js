import { createCommentVNode as e, createElementBlock as t, createTextVNode as n, defineComponent as r, normalizeClass as i, openBlock as a, renderSlot as o, toDisplayString as s } from "vue";
//#region src/components/divider/ori-divider.vue?vue&type=script&setup=true&lang.ts
var c = ["aria-orientation"], l = {
	key: 0,
	class: "ori-divider__label"
}, u = /*@__PURE__*/ r({
	__name: "ori-divider",
	props: {
		color: {},
		text: {},
		vertical: { type: Boolean }
	},
	setup(r) {
		return (u, d) => (a(), t("div", {
			class: i(["ori-divider", {
				"ori-divider_vertical": r.vertical,
				"ori-divider_text": r.text || u.$slots.default,
				[`ori-color_${r.color}`]: r.color
			}]),
			role: "separator",
			"aria-orientation": r.vertical ? "vertical" : void 0
		}, [r.text || u.$slots.default ? (a(), t("span", l, [o(u.$slots, "default", {}, () => [n(s(r.text), 1)])])) : e("", !0)], 10, c));
	}
});
//#endregion
export { u as default };

//# sourceMappingURL=ori-divider.vue_vue_type_script_setup_true_lang.js.map