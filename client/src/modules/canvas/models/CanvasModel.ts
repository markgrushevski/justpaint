abstract class CanvasHandlerModel {
    protected constructor(canvas: HTMLCanvasElement) {
        this.canvas = canvas
    }

    protected canvas: HTMLCanvasElement

    public get canvasDataURL(): string {
        return this.canvas.toDataURL('image/png', 1)
    }
}

export abstract class CanvasHistoryModel extends CanvasHandlerModel {
    protected constructor(canvas: HTMLCanvasElement) {
        super(canvas)
    }
}

export abstract class CanvasToolModel extends CanvasHandlerModel {
    protected abstract eventHandlersMap?: Record<string, (ev: never) => void>

    protected constructor(canvas: HTMLCanvasElement) {
        super(canvas)

        this.ctx = canvas.getContext('2d') as CanvasRenderingContext2D
        if (!this.ctx) throw new Error('CanvasRenderingContext2D not found')
    }

    protected ctx: CanvasRenderingContext2D
}
