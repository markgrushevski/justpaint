import type { ActionSize, RadiusSize, ThemeColor, Variant } from '../../types';
type __VLS_Props = {
    appendIcon?: string;
    closable?: boolean;
    closeLabel?: string;
    color?: ThemeColor;
    disabled?: boolean;
    prependIcon?: string;
    radius?: RadiusSize;
    size?: ActionSize;
    text?: string;
    variant?: Variant;
};
declare var __VLS_6: {};
type __VLS_Slots = {} & {
    default?: (props: typeof __VLS_6) => any;
};
declare const __VLS_base: import("vue").DefineComponent<__VLS_Props, {}, {}, {}, {}, import("vue").ComponentOptionsMixin, import("vue").ComponentOptionsMixin, {
    close: () => any;
}, string, import("vue").PublicProps, Readonly<__VLS_Props> & Readonly<{
    onClose?: (() => any) | undefined;
}>, {}, {}, {}, {}, string, import("vue").ComponentProvideOptions, false, {}, any>;
declare const __VLS_export: __VLS_WithSlots<typeof __VLS_base, __VLS_Slots>;
declare const _default: typeof __VLS_export;
export default _default;
type __VLS_WithSlots<T, S> = T & {
    new (): {
        $slots: S;
    };
};
