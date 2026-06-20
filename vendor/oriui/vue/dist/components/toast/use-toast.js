import { reactive as e } from "vue";
//#region src/components/toast/use-toast.ts
var t = e([]), n = /* @__PURE__ */ new Map(), r = 0;
function i(e) {
	let r = t.findIndex((t) => t.id === e);
	r !== -1 && t.splice(r, 1);
	let i = n.get(e);
	i !== void 0 && (clearTimeout(i), n.delete(e));
}
function a() {
	t.splice(0), n.forEach((e) => clearTimeout(e)), n.clear();
}
function o(e, a) {
	let o = typeof e == "string" ? { text: e } : { ...e }, s = ++r, c = {
		id: s,
		duration: 4e3,
		closable: !0,
		color: a,
		...o
	};
	return t.push(c), c.duration && c.duration > 0 && n.set(s, setTimeout(() => i(s), c.duration)), s;
}
function s() {
	return {
		toasts: t,
		toast: (e) => o(e),
		success: (e) => o(e, "success"),
		error: (e) => o(e, "danger"),
		warn: (e) => o(e, "warn"),
		info: (e) => o(e, "info"),
		dismiss: i,
		clear: a
	};
}
//#endregion
export { s as useToast };

//# sourceMappingURL=use-toast.js.map