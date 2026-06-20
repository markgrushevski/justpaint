import { createBlock as e, defineComponent as t, normalizeClass as n, normalizeStyle as r, openBlock as i, renderSlot as a, resolveDynamicComponent as o, withCtx as s } from "vue";
//#region src/components/stack/ori-stack.vue?vue&type=script&setup=true&lang.ts
var c = /*@__PURE__*/ t({
	__name: "ori-stack",
	props: {
		align: {},
		as: { default: "div" },
		cluster: { type: Boolean },
		gap: {},
		justify: {}
	},
	setup(t) {
		return (c, l) => (i(), e(o(t.as), {
			class: n([t.cluster ? "ori-cluster" : "ori-stack", { [`ori-size-gap_${t.gap}`]: t.gap }]),
			style: r({
				alignItems: t.align,
				justifyContent: t.justify
			})
		}, {
			default: s(() => [a(c.$slots, "default")]),
			_: 3
		}, 8, ["class", "style"]));
	}
});
//#endregion
export { c as default };

//# sourceMappingURL=ori-stack.vue_vue_type_script_setup_true_lang.js.map