export abstract class CanvasEventHandler {
    protected constructor() {}

    protected abstract handleMouseDown(ev: MouseEvent): void
    protected abstract handleMouseMove(ev: MouseEvent): void
    protected abstract handleMouseLeave(ev: MouseEvent): void
    protected abstract handleMouseUp(ev: MouseEvent): void

    protected abstract canvas: HTMLCanvasElement
    protected abstract ctx: CanvasRenderingContext2D
    protected abstract eventHandlersMap: {
        [P in keyof GlobalEventHandlersEventMap]?: (ev: GlobalEventHandlersEventMap[P]) => void
    }
}
