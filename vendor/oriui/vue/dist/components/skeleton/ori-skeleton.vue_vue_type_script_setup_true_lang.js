import { createBlock as e, defineComponent as t, normalizeClass as n, openBlock as r, resolveDynamicComponent as i } from "vue";
//#region src/components/skeleton/ori-skeleton.vue?vue&type=script&setup=true&lang.ts
var a = /*@__PURE__*/ t({
	__name: "ori-skeleton",
	props: {
		as: { default: "div" },
		radius: { default: "sm" }
	},
	setup(t) {
		return (a, o) => (r(), e(i(t.as), {
			class: n(["ori-skeleton", { [`ori-size-radius_${t.radius}`]: t.radius }]),
			"aria-hidden": "true"
		}, null, 8, ["class"]));
	}
});
//#endregion
export { a as default };

