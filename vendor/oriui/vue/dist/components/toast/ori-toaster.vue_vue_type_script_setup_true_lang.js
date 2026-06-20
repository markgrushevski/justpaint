import e from "./ori-toast.js";
import { useToast as t } from "./use-toast.js";
import { Fragment as n, Teleport as r, TransitionGroup as i, createBlock as a, createCommentVNode as o, createElementBlock as s, createVNode as c, defineComponent as l, normalizeClass as u, onMounted as d, openBlock as f, ref as p, renderList as m, unref as h, withCtx as g } from "vue";
//#region src/components/toast/ori-toaster.vue?vue&type=script&setup=true&lang.ts
var _ = /*@__PURE__*/ l({
	__name: "ori-toaster",
	props: { position: { default: "top-right" } },
	setup(l) {
		let { toasts: _, dismiss: v } = t(), y = p(!1);
		return d(() => y.value = !0), (t, d) => y.value ? (f(), a(r, {
			key: 0,
			to: "body"
		}, [c(i, {
			tag: "div",
			name: "ori-toast",
			class: u(["ori-toaster", `ori-toaster_${l.position}`])
		}, {
			default: g(() => [(f(!0), s(n, null, m(h(_), (t) => (f(), a(e, {
				key: t.id,
				closable: t.closable,
				color: t.color,
				icon: t.icon,
				text: t.text,
				title: t.title,
				onClose: (e) => h(v)(t.id)
			}, null, 8, [
				"closable",
				"color",
				"icon",
				"text",
				"title",
				"onClose"
			]))), 128))]),
			_: 1
		}, 8, ["class"])])) : o("", !0);
	}
});
//#endregion
export { _ as default };

//# sourceMappingURL=ori-toaster.vue_vue_type_script_setup_true_lang.js.map