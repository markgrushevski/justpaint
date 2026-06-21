import { type ComputedRef, type InjectionKey } from 'vue';
import type { ActionSize } from '../../types';
export interface OriFieldContext {
    /** The id shared by the field's `<label for>` and the control. */
    id: ComputedRef<string>;
    /** `aria-describedby` pointing at the rendered hint or error (or undefined when neither shows). */
    describedBy: ComputedRef<string | undefined>;
    /** Whether the field is invalid (an `error` is set, or `invalid` was passed). */
    invalid: ComputedRef<boolean>;
    /** Whether the field is required (drives the control's native `required`). */
    required: ComputedRef<boolean>;
    /** Whether the field is disabled (drives the control's native `disabled`). */
    disabled: ComputedRef<boolean>;
    /** The field's action-size, propagated so the control matches the label/hint scale. */
    size: ComputedRef<ActionSize>;
}
export declare const oriFieldKey: InjectionKey<OriFieldContext>;
/**
 * Read the surrounding `OriField` context, if any. Text controls (OriInput / OriSelect / OriTextarea)
 * call this to auto-wire when nested inside an `OriField`; returns `undefined` when used standalone.
 */
export declare function useOriField(): OriFieldContext | undefined;
