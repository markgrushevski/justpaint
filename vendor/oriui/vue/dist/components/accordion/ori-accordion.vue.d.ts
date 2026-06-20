import type { RadiusSize, ThemeColor } from '../../types';
interface AccordionItem {
    value: string | number;
    title: string;
    disabled?: boolean;
}
type __VLS_Props = {
    color?: ThemeColor;
    items: AccordionItem[];
    multiple?: boolean;
    radius?: RadiusSize;
};
declare var __VLS_1: {
    item: AccordionItem;
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
//# sourceMappingURL=ori-accordion.vue.d.ts.map