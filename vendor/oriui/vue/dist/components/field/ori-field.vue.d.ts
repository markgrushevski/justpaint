import type { ActionSize } from '../../types';
type __VLS_Props = {
    /** Extra element id(s) to append to aria-describedby (e.g. a shared form note). */
    describedby?: string;
    disabled?: boolean;
    /** Error message: rendered below the control (role=alert) and flips the field to aria-invalid. */
    error?: string;
    fluid?: boolean;
    /** Helper text below the control; hidden while an error is shown. */
    hint?: string;
    id?: string;
    invalid?: boolean;
    label?: string;
    required?: boolean;
    size?: ActionSize;
};
declare var __VLS_1: {
    id: string;
    invalid: boolean;
    describedby: string | undefined;
    controlAttrs: {
        id: string;
        'aria-describedby': string | undefined;
        'aria-invalid': string | undefined;
        disabled: true | undefined;
        required: true | undefined;
    };
};
type __VLS_Slots = {} & {
    default?: (props: typeof __VLS_1) => any;
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
