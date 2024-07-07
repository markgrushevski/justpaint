import { CanvasToolModel } from './models'

export type Tool = Pen | Eraser | Line | Circle | Triangle | Square

export abstract class CanvasTool extends CanvasToolModel {
    protected static name = 'CanvasTool'

    protected abstract drawStartHandler(ev: MouseEvent): void
    protected abstract drawHandler(ev: MouseEvent): void
    protected abstract drawEndHandler(ev: MouseEvent): void

    protected constructor(canvas: HTMLCanvasElement) {
        super(canvas)

        this.ctx.lineCap = 'round'
        this.ctx.lineJoin = 'round'
        this.ctx.imageSmoothingEnabled = true
        this.ctx.imageSmoothingQuality = 'high'

        this.destroy()
        this.listen()

        // todo: for test
        this.ctx.strokeStyle = '#ff8020'
        this.ctx.fillStyle = '#8020ff'
    }

    protected mouseDown = false

    protected eventHandlersMap: {
        [P in keyof GlobalEventHandlersEventMap]?: (ev: GlobalEventHandlersEventMap[P]) => void
    } = {
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

    protected drawData = {
        start: {
            x: 0,
            y: 0,
            canvasDataURL: ''
        },
        current: {
            x: 0,
            y: 0,
            width: 0,
            height: 0
        }
    }

    protected handleMouseDown(ev: MouseEvent) {
        this.mouseDown = true

        this.drawData.start.canvasDataURL = this.canvas.toDataURL()

        this.drawData.start.x = this.drawData.current.x
        this.drawData.start.y = this.drawData.current.y

        return this.drawStartHandler(ev)
    }

    protected handleMouseMove(ev: MouseEvent) {
        this.drawData.current.x = ev.pageX - (ev.target as HTMLCanvasElement).offsetLeft
        this.drawData.current.y = ev.pageY - (ev.target as HTMLCanvasElement).offsetTop

        this.drawData.current.width = this.drawData.current.x - this.drawData.start.x
        this.drawData.current.height = this.drawData.current.y - this.drawData.start.y

        if (this.mouseDown) {
            return this.drawHandler(ev)
        }
    }

    protected handleMouseLeave(ev: MouseEvent) {
        this.mouseDown = false

        return this.drawEndHandler(ev)
    }

    protected handleMouseUp(ev: MouseEvent) {
        this.mouseDown = false

        return this.drawEndHandler(ev)
    }

    protected figureDrawHandler(cb: () => void) {
        const image = new Image()
        image.src = this.drawData.start.canvasDataURL
        image.onload = () => {
            this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height)
            this.ctx.drawImage(image, 0, 0, this.canvas.width, this.canvas.height)
            cb()
        }
    }

    protected listen() {
        Object.entries(this.eventHandlersMap).forEach(([eventName, listener]) => {
            // @ts-expect-error expect that keys must be in canvas element
            this.canvas[`on${eventName}`] = listener.bind(this)
        })
    }

    protected destroy() {
        Object.entries(this.eventHandlersMap).forEach(([eventName]) => {
            // @ts-expect-error expect that keys must be in canvas element
            this.canvas[`on${eventName}`] = null
        })
    }

    public get strokeColor(): string {
        return this.ctx.strokeStyle as string
    }

    public set strokeColor(color: string) {
        this.ctx.strokeStyle = color
    }

    public get fillColor(): string {
        return this.ctx.fillStyle as string
    }

    public set fillColor(color: string) {
        this.ctx.fillStyle = color
    }

    public get lineWeight(): number {
        return this.ctx.lineWidth
    }

    public set lineWeight(number: number) {
        this.ctx.lineWidth = number
    }
}

export class Pen extends CanvasTool {
    public static readonly name = 'Pen'

    public constructor(canvas: HTMLCanvasElement) {
        super(canvas)
    }

    protected drawStartHandler() {
        this.ctx.beginPath()
        this.ctx.moveTo(this.drawData.current.x, this.drawData.current.y)
    }

    protected drawHandler() {
        this.ctx.lineTo(this.drawData.current.x, this.drawData.current.y)
        this.ctx.stroke()
    }

    protected drawEndHandler() {}
}

export class Eraser extends CanvasTool {
    public static readonly name = 'Eraser'

    public constructor(canvas: HTMLCanvasElement) {
        super(canvas)
    }

    protected drawStartHandler() {
        this.ctx.save()
        this.ctx.beginPath()
        this.ctx.moveTo(this.drawData.current.x, this.drawData.current.y)
    }

    protected drawHandler() {
        this.ctx.lineTo(this.drawData.current.x, this.drawData.current.y)
        this.ctx.stroke()

        this.strokeColor = '#ffffff'
    }

    protected drawEndHandler() {
        this.ctx.restore()
    }
}

export class Line extends CanvasTool {
    public static readonly name = 'Line'

    public constructor(canvas: HTMLCanvasElement) {
        super(canvas)
    }

    protected drawStartHandler() {
        this.ctx.beginPath()
        this.ctx.moveTo(this.drawData.start.x, this.drawData.start.y)
    }

    protected drawHandler() {
        this.figureDrawHandler(() => {
            this.ctx.beginPath()
            this.ctx.moveTo(this.drawData.start.x, this.drawData.start.y)
            this.ctx.lineTo(this.drawData.current.x, this.drawData.current.y)
            this.ctx.stroke()
        })
    }

    protected drawEndHandler() {}
}

export class Circle extends CanvasTool {
    public static readonly name = 'Circle'

    public constructor(canvas: HTMLCanvasElement) {
        super(canvas)
    }

    protected drawStartHandler() {
        this.ctx.beginPath()
    }

    protected drawHandler() {
        const centerX = this.drawData.start.x + this.drawData.current.width / 2
        const centerY = this.drawData.start.y + this.drawData.current.height / 2

        const radiusX = Math.abs(this.drawData.current.width) / 2
        const radiusY = Math.abs(this.drawData.current.height) / 2

        this.figureDrawHandler(() => {
            this.ctx.beginPath()
            this.ctx.ellipse(centerX, centerY, radiusX, radiusY, 0, 0, Math.PI * 2)
            this.ctx.fill()
            this.ctx.stroke()
        })
    }

    protected drawEndHandler() {}
}

export class Triangle extends CanvasTool {
    public static readonly name = 'Triangle'

    public constructor(canvas: HTMLCanvasElement) {
        super(canvas)
    }

    protected drawStartHandler() {
        this.ctx.beginPath()
    }

    protected drawHandler() {
        this.figureDrawHandler(() => {
            this.ctx.beginPath()
            this.ctx.rect(
                this.drawData.start.x,
                this.drawData.start.y,
                this.drawData.current.width,
                this.drawData.current.height
            )
            this.ctx.fill()
            this.ctx.stroke()
        })
    }

    protected drawEndHandler() {}
}

export class Square extends CanvasTool {
    public static readonly name = 'Square'

    public constructor(canvas: HTMLCanvasElement) {
        super(canvas)
    }

    protected drawStartHandler() {
        this.ctx.beginPath()
    }

    protected drawHandler() {
        this.figureDrawHandler(() => {
            this.ctx.beginPath()
            this.ctx.rect(
                this.drawData.start.x,
                this.drawData.start.y,
                this.drawData.current.width,
                this.drawData.current.height
            )
            this.ctx.fill()
            this.ctx.stroke()
        })
    }

    protected drawEndHandler() {}

    /* #prevStartX: number = 0;
    #prevStartY: number = 0;
    
    #prevEndW: number = 0;
    #prevEndH: number = 0;
    
    protected _draw(x: number, y: number, w: number, h: number) {
        this.ctx.clearRect(this.#prevStartX, this.#prevStartY, this.#prevEndW, this.#prevEndH);
        this.ctx.beginPath();
        this.ctx.rect(x, y, w, h);
        this.ctx.fill();
        this.#prevStartX = x;
        this.#prevStartY = y;
        this.#prevEndW = w;
        this.#prevEndH = h;
    } */
}
