import type { ThemeColor } from '../../types';
export interface ToastOptions {
    /** Show a dismiss button on the toast. */
    closable?: boolean;
    /** Semantic color role — drives the accent and the live-region assertiveness. */
    color?: ThemeColor;
    /** Auto-dismiss delay in ms; `0` keeps the toast until it is dismissed. */
    duration?: number;
    /** SVG path for a leading icon. */
    icon?: string;
    /** Body message. */
    text?: string;
    /** Optional bold heading above the text. */
    title?: string;
}
export interface ToastItem extends ToastOptions {
    id: number;
}
declare function dismiss(id: number): void;
declare function clear(): void;
/**
 * Imperative toast queue. Call `toast()` (or a severity shortcut) from anywhere to push a
 * notification, and render `<OriToaster />` once near the app root to display the queue. Each
 * push returns the toast id, which can be passed to `dismiss(id)`.
 */
export declare function useToast(): {
    /** The reactive queue, rendered by `<OriToaster>`. */
    toasts: import("vue").Reactive<ToastItem[]>;
    toast: (options: ToastOptions | string) => number;
    success: (options: ToastOptions | string) => number;
    error: (options: ToastOptions | string) => number;
    warn: (options: ToastOptions | string) => number;
    info: (options: ToastOptions | string) => number;
    dismiss: typeof dismiss;
    clear: typeof clear;
};
export {};
//# sourceMappingURL=use-toast.d.ts.map