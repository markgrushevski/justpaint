import type { ActionSize, RadiusSize, ThemeColor } from '../../types';
interface SelectOption {
    label: string;
    value: string | number;
    disabled?: boolean;
}
type __VLS_Props = {
    color?: ThemeColor;
    /** Extra element id(s) to append to aria-describedby (e.g. a shared form note). */
    describedby?: string;
    disabled?: boolean;
    /** Error message: rendered below the control (role=alert) and flips it to aria-invalid. */
    error?: string;
    fluid?: boolean;
    /** Helper text below the control; hidden while an error is shown. */
    hint?: string;
    id?: string;
    invalid?: boolean;
    label?: string;
    options?: SelectOption[];
    placeholder?: string;
    radius?: RadiusSize;
    required?: boolean;
    size?: ActionSize;
};
type __VLS_ModelProps = {
    modelValue?: string | number;
};
type __VLS_PublicProps = __VLS_Props & __VLS_ModelProps;
declare var __VLS_1: {};
type __VLS_Slots = {} & {
    default?: (props: typeof __VLS_1) => any;
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
