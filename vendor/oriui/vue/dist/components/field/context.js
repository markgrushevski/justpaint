import { inject as e } from "vue";
//#region src/components/field/context.ts
var t = Symbol("ori-field");
function n() {
	return e(t, void 0);
}
//#endregion
export { t as oriFieldKey, n as useOriField };

