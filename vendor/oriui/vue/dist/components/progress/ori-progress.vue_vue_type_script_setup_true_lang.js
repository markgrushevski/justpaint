import { computed as e, createElementBlock as t, createElementVNode as n, defineComponent as r, normalizeClass as i, normalizeStyle as a, openBlock as o } from "vue";
//#region src/components/progress/ori-progress.vue?vue&type=script&setup=true&lang.ts
var s = [
	"aria-label",
	"aria-valuemax",
	"aria-valuenow"
], c = { class: "ori-progress__track" }, l = ["data-indeterminate"], u = /*@__PURE__*/ r({
	__name: "ori-progress",
	props: {
		color: { default: "primary" },
		indeterminate: {
			type: Boolean,
			default: !1
		},
		label: { default: "Loading" },
		max: { default: 100 },
		radius: { default: "rounded" },
		size: { default: "md" },
		value: { default: 0 }
	},
	setup(r) {
		let u = e(() => Math.min(Math.max(r.value, 0), r.max)), d = e(() => r.max > 0 ? u.value / r.max * 100 : 0);
		return (e, f) => (o(), t("div", {
			class: i([
				"ori-progress",
				`ori-progress_${r.size}`,
				{
					[`ori-size-radius_${r.radius}`]: r.radius,
					[`ori-color_${r.color}`]: r.color
				}
			]),
			role: "progressbar",
			"aria-label": r.label,
			"aria-valuemin": 0,
			"aria-valuemax": r.max,
			"aria-valuenow": r.indeterminate ? void 0 : u.value
		}, [n("div", c, [n("div", {
			class: "ori-progress__indicator",
			"data-indeterminate": r.indeterminate ? "" : void 0,
			style: a(r.indeterminate ? void 0 : { width: `${d.value}%` })
		}, null, 12, l)])], 10, s));
	}
});
//#endregion
export { u as default };

//# sourceMappingURL=ori-progress.vue_vue_type_script_setup_true_lang.js.map