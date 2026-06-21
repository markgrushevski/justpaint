import { createBlock as e, createTextVNode as t, defineComponent as n, openBlock as r, renderSlot as i, resolveDynamicComponent as a, toDisplayString as o, withCtx as s } from "vue";
//#region src/components/kbd/ori-kbd.vue?vue&type=script&setup=true&lang.ts
var c = /*@__PURE__*/ n({
	__name: "ori-kbd",
	props: {
		as: { default: "kbd" },
		text: {}
	},
	setup(n) {
		return (c, l) => (r(), e(a(n.as), { class: "ori-kbd" }, {
			default: s(() => [i(c.$slots, "default", {}, () => [t(o(n.text), 1)])]),
			_: 3
		}));
	}
});
//#endregion
export { c as default };

