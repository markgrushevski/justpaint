import { createBlock as e, defineComponent as t, normalizeClass as n, openBlock as r, renderSlot as i, resolveDynamicComponent as a, withCtx as o } from "vue";
//#region src/components/join/ori-join.vue?vue&type=script&setup=true&lang.ts
var s = /*@__PURE__*/ t({
	__name: "ori-join",
	props: {
		as: { default: "div" },
		vertical: { type: Boolean }
	},
	setup(t) {
		return (s, c) => (r(), e(a(t.as), {
			class: n(["ori-join", { "ori-join_vertical": t.vertical }]),
			role: "group"
		}, {
			default: o(() => [i(s.$slots, "default")]),
			_: 3
		}, 8, ["class"]));
	}
});
//#endregion
export { s as default };

