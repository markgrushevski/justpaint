import { createElementBlock as e, createElementVNode as t, createTextVNode as n, defineComponent as r, normalizeClass as i, openBlock as a, renderSlot as o, toDisplayString as s, unref as c, useId as l } from "vue";
//#region src/components/tooltip/ori-tooltip.vue?vue&type=script&setup=true&lang.ts
var u = ["aria-describedby"], d = ["id"], f = /*@__PURE__*/ r({
	__name: "ori-tooltip",
	props: {
		color: {},
		content: {},
		placement: { default: "top" }
	},
	setup(r) {
		let f = l();
		return (l, p) => (a(), e("span", { class: i(["ori-tooltip", r.color && `ori-color_${r.color}`]) }, [t("span", {
			class: "ori-tooltip__trigger",
			"aria-describedby": c(f)
		}, [o(l.$slots, "default", { bubbleId: c(f) })], 8, u), t("span", {
			id: c(f),
			class: i(["ori-tooltip__bubble", `ori-tooltip__bubble_${r.placement}`]),
			role: "tooltip"
		}, [o(l.$slots, "content", {}, () => [n(s(r.content), 1)])], 10, d)], 2));
	}
});
//#endregion
export { f as default };

