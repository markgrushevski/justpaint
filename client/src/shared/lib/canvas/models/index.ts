abstract class CanvasHandlerModel {
    protected constructor(canvas: HTMLCanvasElement) {
        this.canvas = canvas
        this.ctx = canvas.getContext('2d') as CanvasRenderingContext2D
        if (!this.ctx) throw new Error('CanvasRenderingContext2D not found')
    }

    protected canvas: HTMLCanvasElement
    protected ctx: CanvasRenderingContext2D
}

export abstract class CanvasToolModel extends CanvasHandlerModel {
    protected constructor(canvas: HTMLCanvasElement) {
        super(canvas)
    }

    protected abstract eventHandlersMap: {
        [P in keyof GlobalEventHandlersEventMap]?: (
            ev: GlobalEventHandlersEventMap[P],
            cb: (ev: GlobalEventHandlersEventMap[P]) => void
        ) => void
    }
}

export abstract class CanvasHistoryModel extends CanvasHandlerModel {
    protected constructor(canvas: HTMLCanvasElement) {
        super(canvas)
    }
}
