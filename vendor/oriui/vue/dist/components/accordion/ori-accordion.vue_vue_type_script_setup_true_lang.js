import { Fragment as e, computed as t, createElementBlock as n, createElementVNode as r, defineComponent as i, normalizeClass as a, openBlock as o, renderList as s, renderSlot as c, toDisplayString as l, useId as u, withKeys as d } from "vue";
//#region src/components/accordion/ori-accordion.vue?vue&type=script&setup=true&lang.ts
var f = ["name"], p = [
	"aria-disabled",
	"tabindex",
	"onClick",
	"onKeydown"
], m = { class: "ori-accordion__title" }, h = { class: "ori-accordion__panel" }, g = /*@__PURE__*/ i({
	__name: "ori-accordion",
	props: {
		color: { default: "primary" },
		items: {},
		multiple: {
			type: Boolean,
			default: !1
		},
		radius: { default: "md" }
	},
	setup(i) {
		let g = u(), _ = t(() => i.multiple ? void 0 : g);
		function v(e, t) {
			t && e.preventDefault();
		}
		return (t, u) => (o(), n("div", { class: a([
			"ori-accordion",
			`ori-color_${i.color}`,
			{ [`ori-size-radius_${i.radius}`]: i.radius }
		]) }, [(o(!0), n(e, null, s(i.items, (e) => (o(), n("details", {
			key: e.value,
			class: "ori-accordion__item",
			name: _.value
		}, [r("summary", {
			class: "ori-accordion__trigger",
			"aria-disabled": e.disabled ? "true" : void 0,
			tabindex: e.disabled ? -1 : void 0,
			onClick: (t) => v(t, e.disabled),
			onKeydown: [d((t) => v(t, e.disabled), ["enter"]), d((t) => v(t, e.disabled), ["space"])]
		}, [r("span", m, l(e.title), 1), u[0] ||= r("svg", {
			class: "ori-accordion__icon",
			viewBox: "0 0 24 24",
			"aria-hidden": "true",
			xmlns: "http://www.w3.org/2000/svg"
		}, [r("path", { d: "m6 9 6 6 6-6" })], -1)], 40, p), r("div", h, [c(t.$slots, "default", { item: e })])], 8, f))), 128))], 2));
	}
});
//#endregion
export { g as default };

//# sourceMappingURL=ori-accordion.vue_vue_type_script_setup_true_lang.js.map