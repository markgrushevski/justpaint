import { Fragment as e, computed as t, createElementBlock as n, createElementVNode as r, defineComponent as i, mergeModels as a, normalizeClass as o, openBlock as s, ref as c, renderList as l, renderSlot as u, toDisplayString as d, useId as f, useModel as p, watch as m } from "vue";
//#region src/components/tabs/ori-tabs.vue?vue&type=script&setup=true&lang.ts
var h = ["aria-orientation"], g = [
	"id",
	"disabled",
	"aria-selected",
	"aria-controls",
	"tabindex",
	"onClick",
	"onKeydown"
], _ = [
	"id",
	"aria-labelledby",
	"hidden"
], v = /*@__PURE__*/ i({
	__name: "ori-tabs",
	props: /*@__PURE__*/ a({
		color: { default: "primary" },
		orientation: { default: "horizontal" },
		tabs: {}
	}, {
		modelValue: {},
		modelModifiers: {}
	}),
	emits: ["update:modelValue"],
	setup(i) {
		let a = p(i, "modelValue"), v = f(), y = (e) => `${v}-tab-${e}`, b = (e) => `${v}-panel-${e}`, x = c(), S = (e) => {
			x.value?.querySelectorAll("[role=\"tab\"]")[e]?.focus();
		}, C = t(() => i.tabs.find((e) => !e.disabled)?.value), w = t(() => a.value ?? C.value);
		m([() => i.tabs, () => a.value], () => {
			let e = i.tabs.find((e) => e.value === a.value);
			(a.value === void 0 || !e || e.disabled) && C.value !== void 0 && (a.value = C.value);
		}, { immediate: !0 });
		let T = (e) => {
			e.disabled || (a.value = e.value);
		}, E = (e, t) => {
			let n = i.tabs.length;
			if (n !== 0) for (let r = 1; r <= n; r += 1) {
				let a = (e + t * r + n * r) % n, o = i.tabs[a];
				if (o && !o.disabled) {
					T(o), S(a);
					return;
				}
			}
		}, D = (e) => {
			let t = e === "first" ? 0 : i.tabs.length - 1, n = e === "first" ? 1 : -1;
			for (let e = t; e >= 0 && e < i.tabs.length; e += n) {
				let t = i.tabs[e];
				if (t && !t.disabled) {
					T(t), S(e);
					return;
				}
			}
		}, O = (e, t) => {
			let n = i.orientation === "vertical" ? "ArrowDown" : "ArrowRight", r = i.orientation === "vertical" ? "ArrowUp" : "ArrowLeft";
			switch (e.key) {
				case n:
					e.preventDefault(), E(t, 1);
					break;
				case r:
					e.preventDefault(), E(t, -1);
					break;
				case "Home":
					e.preventDefault(), D("first");
					break;
				case "End":
					e.preventDefault(), D("last");
					break;
			}
		};
		return (t, a) => (s(), n("div", { class: o([
			"ori-tabs",
			`ori-color_${i.color}`,
			{ "ori-tabs_vertical": i.orientation === "vertical" }
		]) }, [r("div", {
			ref_key: "tablistRef",
			ref: x,
			class: "ori-tabs__list",
			role: "tablist",
			"aria-orientation": i.orientation
		}, [(s(!0), n(e, null, l(i.tabs, (e, t) => (s(), n("button", {
			id: y(t),
			key: e.value,
			class: "ori-tabs__tab",
			type: "button",
			role: "tab",
			disabled: e.disabled,
			"aria-selected": e.value === w.value ? "true" : "false",
			"aria-controls": b(t),
			tabindex: e.value === w.value ? 0 : -1,
			onClick: (t) => T(e),
			onKeydown: (e) => O(e, t)
		}, d(e.label), 41, g))), 128))], 8, h), (s(!0), n(e, null, l(i.tabs, (e, r) => (s(), n("div", {
			id: b(r),
			key: e.value,
			class: "ori-tabs__panel",
			role: "tabpanel",
			"aria-labelledby": y(r),
			hidden: e.value !== w.value,
			tabindex: "0"
		}, [u(t.$slots, String(e.value), { tab: e }, () => [u(t.$slots, "default", { tab: e })])], 8, _))), 128))], 2));
	}
});
//#endregion
export { v as default };

//# sourceMappingURL=ori-tabs.vue_vue_type_script_setup_true_lang.js.map