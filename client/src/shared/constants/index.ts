import type { InjectionKey, useTemplateRef } from 'vue'

export const injectionKeys = {
    canvasRef: Symbol('canvasRef') as InjectionKey<ReturnType<typeof useTemplateRef<HTMLCanvasElement, string>>>
}
