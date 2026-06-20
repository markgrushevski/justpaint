interface BlockSizes {
    xs: string;
    sm: string;
    md: string;
    lg: string;
    xl: string;
}
interface ScreenSizes extends BlockSizes {
    xxl: string;
}
interface ActionSizes extends BlockSizes {
    text: string;
    xxl: string;
}
interface ActionSpaceSizes extends BlockSizes {
    text: string;
    xxl: string;
}
interface GapSizes extends BlockSizes {
    zero: string;
}
interface RadiusSizes extends BlockSizes {
    zero: string;
    rounded: string;
}
export type Sizes = BlockSizes & ScreenSizes & ActionSizes & ActionSpaceSizes & GapSizes & RadiusSizes;
export type BlockSize = keyof BlockSizes;
export type ScreenSize = keyof ScreenSizes;
export type ActionSize = keyof ActionSizes;
export type ActionSpaceSize = keyof ActionSpaceSizes;
export type GapSize = keyof GapSizes;
export type RadiusSize = keyof RadiusSizes;
export type Size = keyof Sizes;
export type CenterPosition = 'center';
export type InlinePosition = 'right' | 'left';
export type BlockPosition = 'top' | 'bottom';
export type CustomPosition = 'top-left' | 'top-right' | 'bottom-left' | 'bottom-right';
export type CenteredPosition = BlockPosition | InlinePosition;
export type Position = CenterPosition | CenteredPosition | CustomPosition;
interface SeverityColors {
    success: string;
    warn: string;
    danger: string;
    info: string;
}
interface ThemeColors extends SeverityColors {
    primary: string;
    secondary: string;
    surface: string;
    background: string;
}
export type SeverityColor = keyof SeverityColors;
export type ThemeColor = keyof ThemeColors;
export type Variant = 'fill' | 'tonal' | 'outline' | 'text' | 'plain';
/** Library utility type. Recursively makes all properties partial. */
export type DeepPartial<T> = T extends object ? {
    [P in keyof T]?: DeepPartial<T[P]>;
} : T;
export {};
//# sourceMappingURL=types.d.ts.map