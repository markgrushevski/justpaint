import type { ThemeColor } from '../../types';
type __VLS_Props = {
    color?: ThemeColor;
    /** Tooltip text. For rich content use the #content slot instead (it takes precedence). */
    content?: string;
    placement?: 'top' | 'bottom' | 'left' | 'right';
};
declare var __VLS_1: {
    bubbleId: string;
}, __VLS_3: {};
type __VLS_Slots = {} & {
    default?: (props: typeof __VLS_1) => any;
} & {
    content?: (props: typeof __VLS_3) => any;
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
