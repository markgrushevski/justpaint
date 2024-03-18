import { CanvasEventHandlerModel, CanvasWorkModel } from './models'

export class CanvasWork extends CanvasWorkModel {
    public constructor(canvas: HTMLCanvasElement) {
        super()

        this.canvas = canvas
        this.ctx = canvas.getContext('2d') as CanvasRenderingContext2D
        if (!this.ctx) throw new Error('CanvasRenderingContext2D not found')

        this.destroy()
        this.listen()
    }

    private actionHistoryList = []

    private loadStateToCanvas() {}

    private makeActionHistoryStep() {}

    protected canvas: HTMLCanvasElement
    protected ctx: CanvasRenderingContext2D

    protected eventHandlersMap = {
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

    protected handleMouseDown(ev: MouseEvent): void {}

    protected handleMouseLeave(ev: MouseEvent): void {}

    protected handleMouseMove(ev: MouseEvent): void {}

    protected handleMouseUp(ev: MouseEvent): void {}

    protected listen() {
        Object.entries(this.eventHandlersMap).forEach(([eventName, listener]) => {
            // @ts-expect-error expect that keys must be in canvas instance
            //this.canvas[`on${ eventName }`] = listener.bind(this)
        })
    }

    protected destroy() {
        Object.entries(this.eventHandlersMap).forEach(([eventName]) => {
            // @ts-expect-error expect that keys must be in canvas instance
            //this.canvas[`on${ eventName }`] = null
        })
    }

    public undo() {}
    public redo() {}
    public save() {}
    public load() {}
}
