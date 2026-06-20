import { createBlock as e, defineComponent as t, normalizeClass as n, openBlock as r, renderSlot as i, resolveDynamicComponent as a, withCtx as o } from "vue";
//#region src/components/link/ori-link.vue?vue&type=script&setup=true&lang.ts
var s = /*@__PURE__*/ t({
	__name: "ori-link",
	props: {
		as: { default: "a" },
		color: {},
		external: { type: Boolean },
		hover: { type: Boolean },
		href: {}
	},
	setup(t) {
		return (s, c) => (r(), e(a(t.as), {
			class: n(["ori-link", {
				"ori-link_hover": t.hover,
				[`ori-color_${t.color}`]: t.color
			}]),
			href: t.href,
			target: t.external ? "_blank" : void 0,
			rel: t.external ? "noopener noreferrer" : void 0
		}, {
			default: o(() => [i(s.$slots, "default")]),
			_: 3
		}, 8, [
			"class",
			"href",
			"target",
			"rel"
		]));
	}
});
//#endregion
export { s as default };

//# sourceMappingURL=ori-link.vue_vue_type_script_setup_true_lang.js.map