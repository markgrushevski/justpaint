import type { ActionSize, ThemeColor } from '../../types';
interface RadioOption {
    label: string;
    value: string | number;
    disabled?: boolean;
}
type __VLS_Props = {
    color?: ThemeColor;
    disabled?: boolean;
    inline?: boolean;
    label?: string;
    /** Shared radio `name`; auto-generated (useId) when omitted. */
    name?: string;
    options?: RadioOption[];
    required?: boolean;
    size?: ActionSize;
};
type __VLS_ModelProps = {
    modelValue?: string | number;
};
type __VLS_PublicProps = __VLS_Props & __VLS_ModelProps;
declare const __VLS_export: import("vue").DefineComponent<__VLS_PublicProps, {}, {}, {}, {}, import("vue").ComponentOptionsMixin, import("vue").ComponentOptionsMixin, {
    "update:modelValue": (value: string | number | undefined) => any;
}, string, import("vue").PublicProps, Readonly<__VLS_PublicProps> & Readonly<{
    "onUpdate:modelValue"?: ((value: string | number | undefined) => any) | undefined;
}>, {}, {}, {}, {}, string, import("vue").ComponentProvideOptions, false, {}, any>;
declare const _default: typeof __VLS_export;
export default _default;
