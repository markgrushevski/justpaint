import { createElementBlock as e, defineComponent as t, normalizeClass as n, openBlock as r } from "vue";
//#region src/components/spinner/ori-spinner.vue?vue&type=script&setup=true&lang.ts
var i = ["aria-label"], a = /*@__PURE__*/ t({
	__name: "ori-spinner",
	props: {
		color: {},
		inline: { type: Boolean },
		label: { default: "Loading" },
		size: { default: "text" }
	},
	setup(t) {
		return (a, o) => (r(), e("div", {
			class: n(["ori-spinner", {
				"ori-spinner_inline": t.inline,
				[`ori-spinner_${t.size}`]: t.size,
				[`ori-color_${t.color}`]: t.color
			}]),
			role: "status",
			"aria-label": t.label
		}, null, 10, i));
	}
});
//#endregion
export { a as default };

