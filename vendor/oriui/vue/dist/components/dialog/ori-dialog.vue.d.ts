type __VLS_Props = {
    closeOnEscape?: boolean;
    closeOnInteractOutside?: boolean;
    defaultOpen?: boolean;
    modal?: boolean;
    title?: string;
};
declare var __VLS_1: {
    props: Record<string, unknown>;
    open: boolean;
}, __VLS_3: {}, __VLS_5: {};
type __VLS_Slots = {} & {
    trigger?: (props: typeof __VLS_1) => any;
} & {
    title?: (props: typeof __VLS_3) => any;
} & {
    default?: (props: typeof __VLS_5) => any;
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
//# sourceMappingURL=ori-dialog.vue.d.ts.map