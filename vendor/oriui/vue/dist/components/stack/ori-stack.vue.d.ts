import type { GapSize } from '../../types';
type __VLS_Props = {
    /** CSS `align-items` value (cross-axis alignment). */
    align?: string;
    /** An HTML tag name, a Component name or Component class reference. */
    as?: string | object;
    /** Wrapping row (cluster) instead of a column stack. */
    cluster?: boolean;
    gap?: GapSize;
    /** CSS `justify-content` value (main-axis distribution). */
    justify?: string;
};
declare var __VLS_8: {};
type __VLS_Slots = {} & {
    default?: (props: typeof __VLS_8) => any;
};
declare const __VLS_base: import("vue").DefineComponent<__VLS_Props, {}, {}, {}, {}, import("vue").ComponentOptionsMixin, import("vue").ComponentOptionsMixin, {}, string, import("vue").PublicProps, Readonly<__VLS_Props> & Readonly<{}>, {}, {}, {}, {}, string, import("vue").ComponentProvideOptions, false, {}, any>;
declare const __VLS_export: __VLS_WithSlots<typeof __VLS_base, __VLS_Slots>;
declare const _default: typeof __VLS_export;
export default _default;
type __VLS_WithSlots<T, S> = T & {
    new (): {
        $slots: S;
    };
};
//# sourceMappingURL=ori-stack.vue.d.ts.map