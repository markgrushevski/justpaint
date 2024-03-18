abstract class CanvasHandler {
    protected constructor(canvas: HTMLCanvasElement) {
        this.canvas = canvas
    }

    protected canvas: HTMLCanvasElement
}

export abstract class CanvasEventHandlerModel extends CanvasHandler {
    protected constructor(canvas: HTMLCanvasElement) {
        super(canvas)
    }
}

export abstract class CanvasToolModel extends CanvasHandler {
    protected constructor(canvas: HTMLCanvasElement) {
        super(canvas)

        this.ctx = canvas.getContext('2d') as CanvasRenderingContext2D
        if (!this.ctx) throw new Error('CanvasRenderingContext2D not found')

        console.log('Tool', this.ctx.lineCap)
    }

    protected ctx: CanvasRenderingContext2D
}

export abstract class CanvasWorkModel extends CanvasHandler {
    protected constructor(canvas: HTMLCanvasElement) {
        super(canvas)

        this.ctx = canvas.getContext('2d') as CanvasRenderingContext2D
        if (!this.ctx) throw new Error('CanvasRenderingContext2D not found')

        console.log('Work', this.ctx.lineCap)
    }

    protected ctx: CanvasRenderingContext2D
}
