import type { RadiusSize, ThemeColor, Variant } from '../../types';
type __VLS_Props = {
    appendAvatar?: string;
    appendIcon?: string;
    color?: ThemeColor;
    disabled?: boolean;
    fluid?: boolean;
    image?: string;
    loading?: boolean;
    prependAvatar?: string;
    prependIcon?: string;
    radius?: RadiusSize;
    reverseAppendedActions?: boolean;
    reversePrependedActions?: boolean;
    subtitle?: string;
    text?: string;
    title?: string;
    variant?: Variant;
    row?: boolean;
};
declare var __VLS_1: {}, __VLS_3: {}, __VLS_5: {}, __VLS_17: {}, __VLS_19: {}, __VLS_21: {}, __VLS_33: {}, __VLS_35: {};
type __VLS_Slots = {} & {
    default?: (props: typeof __VLS_1) => any;
} & {
    'actions-prepend'?: (props: typeof __VLS_3) => any;
} & {
    'header-prepend'?: (props: typeof __VLS_5) => any;
} & {
    title?: (props: typeof __VLS_17) => any;
} & {
    subtitle?: (props: typeof __VLS_19) => any;
} & {
    'header-append'?: (props: typeof __VLS_21) => any;
} & {
    body?: (props: typeof __VLS_33) => any;
} & {
    'actions-append'?: (props: typeof __VLS_35) => any;
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
//# sourceMappingURL=ori-card.vue.d.ts.map