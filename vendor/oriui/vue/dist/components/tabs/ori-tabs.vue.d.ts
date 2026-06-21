import type { ThemeColor } from '../../types';
interface TabItem {
    value: string | number;
    label: string;
    disabled?: boolean;
}
type __VLS_Props = {
    /** Active-tab accent (indicator + focus ring). */
    color?: ThemeColor;
    orientation?: 'horizontal' | 'vertical';
    tabs: TabItem[];
};
type __VLS_ModelProps = {
    modelValue?: string | number;
};
type __VLS_PublicProps = __VLS_Props & __VLS_ModelProps;
declare var __VLS_2: string, __VLS_3: {
    tab: TabItem;
}, __VLS_5: {
    tab: TabItem;
};
type __VLS_Slots = {} & {
    [K in NonNullable<typeof __VLS_2>]?: (props: typeof __VLS_3) => any;
} & {
    default?: (props: typeof __VLS_5) => any;
};
declare const __VLS_base: import("vue").DefineComponent<__VLS_PublicProps, {}, {}, {}, {}, import("vue").ComponentOptionsMixin, import("vue").ComponentOptionsMixin, {
    "update:modelValue": (value: string | number | undefined) => any;
}, string, import("vue").PublicProps, Readonly<__VLS_PublicProps> & Readonly<{
    "onUpdate:modelValue"?: ((value: string | number | undefined) => any) | undefined;
}>, {}, {}, {}, {}, string, import("vue").ComponentProvideOptions, false, {}, any>;
declare const __VLS_export: __VLS_WithSlots<typeof __VLS_base, __VLS_Slots>;
declare const _default: typeof __VLS_export;
export default _default;
type __VLS_WithSlots<T, S> = T & {
    new (): {
        $slots: S;
    };
};
