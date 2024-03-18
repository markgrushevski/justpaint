export class CanvasEventHandler {
    protected static name = 'CanvasEventHandler'

    protected constructor(canvas: HTMLCanvasElement) {
        this.canvas = canvas
    }

    protected abstract handleMouseDown(ev: MouseEvent): void
    protected abstract handleMouseMove(ev: MouseEvent): void
    protected abstract handleMouseLeave(ev: MouseEvent): void
    protected abstract handleMouseUp(ev: MouseEvent): void

    protected abstract canvas: HTMLCanvasElement
    protected abstract ctx: CanvasRenderingContext2D
    protected abstract eventHandlersMap: {
        [P in keyof GlobalEventHandlersEventMap]?: (ev: GlobalEventHandlersEventMap[P]) => void
    }

    public static eventHandlersMap = {
        mousedown: this.handleMouseDown,
        mousemove: this.handleMouseMove,
        mouseleave: this.handleMouseLeave,
        mouseup: this.handleMouseUp
        /* touchmove: null,
        touchstart: null,
        touchend: null,
        touchcancel: null,
        wheel: null */
    }
}
