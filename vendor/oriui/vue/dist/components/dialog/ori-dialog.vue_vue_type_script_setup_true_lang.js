import { Fragment as e, createElementBlock as t, createElementVNode as n, createTextVNode as r, defineComponent as i, mergeProps as a, openBlock as o, renderSlot as s, toDisplayString as c, unref as l, useTemplateRef as u, watchPostEffect as d } from "vue";
import { useDialog as f } from "@oriui/headless/vue";
//#region src/components/dialog/ori-dialog.vue?vue&type=script&setup=true&lang.ts
var p = { class: "ori-dialog__content" }, m = { class: "ori-dialog__header" }, h = { class: "ori-dialog__body" }, g = /*@__PURE__*/ i({
	__name: "ori-dialog",
	props: {
		closeOnEscape: {
			type: Boolean,
			default: !0
		},
		closeOnInteractOutside: {
			type: Boolean,
			default: !0
		},
		defaultOpen: {
			type: Boolean,
			default: !1
		},
		modal: {
			type: Boolean,
			default: !0
		},
		title: {}
	},
	setup(i) {
		let g = f(() => ({
			closeOnEscape: i.closeOnEscape,
			closeOnInteractOutside: i.closeOnInteractOutside,
			defaultOpen: i.defaultOpen,
			modal: i.modal
		})), _ = u("dialog");
		return d(() => {
			let e = _.value;
			e && (g.open.value && !e.open ? i.modal ? e.showModal() : e.show() : !g.open.value && e.open && e.close());
		}), (u, d) => (o(), t(e, null, [s(u.$slots, "trigger", {
			props: l(g).triggerProps.value,
			open: l(g).open.value
		}), n("dialog", a({ ref: "dialog" }, l(g).dialogProps.value, { class: "ori-dialog" }), [n("div", p, [n("header", m, [n("h2", a(l(g).titleProps.value, { class: "ori-dialog__title" }), [s(u.$slots, "title", {}, () => [r(c(i.title), 1)])], 16), n("button", a(l(g).closeTriggerProps.value, {
			type: "button",
			class: "ori-dialog__close",
			"aria-label": "Close"
		}), " × ", 16)]), n("div", h, [s(u.$slots, "default")])])], 16)], 64));
	}
});
//#endregion
export { g as default };

//# sourceMappingURL=ori-dialog.vue_vue_type_script_setup_true_lang.js.map