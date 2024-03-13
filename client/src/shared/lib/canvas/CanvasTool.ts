import { CanvasEventHandler } from './models'

export abstract class CanvasTool extends CanvasEventHandler {
    protected abstract mouseDownHandler(ev: MouseEvent): void

    protected abstract mouseMoveHandler(ev: MouseEvent): void

    protected abstract mouseLeaveHandler(ev: MouseEvent): void

    protected abstract mouseUpHandler(ev: MouseEvent): void

    protected abstract draw(...args: unknown[]): void

    protected static name = 'CanvasTool'

    protected constructor(canvas: HTMLCanvasElement) {
        super()

        this.canvas = canvas
        this.ctx = canvas.getContext('2d') as CanvasRenderingContext2D
        if (!this.ctx) throw new Error('CanvasRenderingContext2D not found')

        this.ctx.lineCap = 'round'
        this.ctx.lineJoin = 'round'
        this.ctx.imageSmoothingEnabled = true
        this.ctx.imageSmoothingQuality = 'high'

        this.mouseDown = false

        this.destroy()
        this.listen()
    }

    protected mouseDown: boolean
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

    protected mousePosition = { x: 0, y: 0 }

    protected handleMouseDown(ev: MouseEvent) {
        this.mouseDown = true
        return this.mouseDownHandler(ev)
    }

    protected handleMouseMove(ev: MouseEvent) {
        this.mousePosition.x = ev.pageX - (ev.target as HTMLCanvasElement).offsetLeft
        this.mousePosition.y = ev.pageY - (ev.target as HTMLCanvasElement).offsetTop
        return this.mouseMoveHandler(ev)
    }

    protected handleMouseLeave(ev: MouseEvent) {
        this.mouseDown = false
        return this.mouseLeaveHandler(ev)
    }

    protected handleMouseUp(ev: MouseEvent) {
        this.mouseDown = false
        return this.mouseUpHandler(ev)
    }

    protected listen() {
        Object.entries(this.eventHandlersMap).forEach(([eventName, listener]) => {
            // @ts-expect-error expect that keys must be in canvas instance
            this.canvas[`on${eventName}`] = listener.bind(this)
        })
    }

    protected destroy() {
        Object.entries(this.eventHandlersMap).forEach(([eventName]) => {
            // @ts-expect-error expect that keys must be in canvas instance
            this.canvas[`on${eventName}`] = null
        })
    }

    public get strokeColor(): typeof this.ctx.strokeStyle {
        return this.ctx.strokeStyle
    }

    public set strokeColor(color: typeof this.ctx.strokeStyle) {
        this.ctx.strokeStyle = color
    }

    public get fillColor(): typeof this.ctx.fillStyle {
        return this.ctx.fillStyle
    }

    public set fillColor(color: typeof this.ctx.fillStyle) {
        this.ctx.fillStyle = color
    }

    public get lineWeight(): typeof this.ctx.lineWidth {
        return this.ctx.lineWidth
    }

    public set lineWeight(number: typeof this.ctx.lineWidth) {
        this.ctx.lineWidth = number
    }
}

export class Pen extends CanvasTool {
    public static name = 'Pen'

    public constructor(canvas: HTMLCanvasElement) {
        super(canvas)
    }

    protected mouseDownHandler() {
        this.ctx.beginPath()
        this.ctx.moveTo(this.mousePosition.x, this.mousePosition.y)
    }

    protected mouseMoveHandler() {
        if (this.mouseDown) {
            this.draw(this.mousePosition.x, this.mousePosition.y)
        }
    }

    protected mouseLeaveHandler() {}

    protected mouseUpHandler() {}

    protected draw(x: number, y: number) {
        this.ctx.lineTo(x, y)
        this.ctx.stroke()
    }
}

export class Eraser extends CanvasTool {
    public static name = 'Eraser'

    public constructor(canvas: HTMLCanvasElement) {
        super(canvas)
    }

    protected mouseDownHandler() {
        this.ctx.save()
        this.ctx.beginPath()
        this.ctx.moveTo(this.mousePosition.x, this.mousePosition.y)
    }

    protected mouseMoveHandler() {
        if (this.mouseDown) {
            this.draw(this.mousePosition.x, this.mousePosition.y)
        }
    }

    protected mouseLeaveHandler() {
        this.ctx.restore()
    }

    protected mouseUpHandler() {
        this.ctx.restore()
    }

    protected draw(x: number, y: number) {
        this.ctx.lineTo(x, y)
        this.ctx.stroke()
        this.strokeColor = '#ffffff'
    }
}

export class Line extends CanvasTool {
    public static name = 'Line'

    public constructor(canvas: HTMLCanvasElement) {
        super(canvas)
    }

    #startX: number = 0
    #startY: number = 0

    #saved: string = ''

    protected mouseDownHandler() {
        this.#saved = this.canvas.toDataURL()
        this.#startX = this.mousePosition.x
        this.#startY = this.mousePosition.y
        this.ctx.beginPath()
        this.ctx.moveTo(this.#startX, this.#startY)
    }

    protected mouseMoveHandler() {
        if (this.mouseDown) {
            this.draw(this.mousePosition.x, this.mousePosition.y)
        }
    }

    protected mouseLeaveHandler() {}

    protected mouseUpHandler() {}

    protected draw(x: number, y: number) {
        const image = new Image()
        image.src = this.#saved
        image.onload = () => {
            this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height)
            this.ctx.drawImage(image, 0, 0, this.canvas.width, this.canvas.height)
            this.ctx.beginPath()
            this.ctx.moveTo(this.#startX, this.#startY)
            this.ctx.lineTo(x, y)
            this.ctx.stroke()
        }
    }
}

export class Circle extends CanvasTool {
    public static name = 'Circle'

    public constructor(canvas: HTMLCanvasElement) {
        super(canvas)
    }

    #startX: number = 0
    #startY: number = 0

    #saved: string = ''

    protected mouseDownHandler() {
        this.#saved = this.canvas.toDataURL()
        this.#startX = this.mousePosition.x
        this.#startY = this.mousePosition.y
        this.ctx.beginPath()
    }

    protected mouseMoveHandler() {
        if (this.mouseDown) {
            const width = this.mousePosition.x - this.#startX
            const height = this.mousePosition.y - this.#startY
            const r = Math.sqrt(width ** 2 + height ** 2)
            this.draw(this.#startX, this.#startY, r)
        }
    }

    protected mouseLeaveHandler() {}

    protected mouseUpHandler() {}

    protected draw(x: number, y: number, r: number) {
        const image = new Image()
        image.src = this.#saved
        image.onload = () => {
            this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height)
            this.ctx.drawImage(image, 0, 0, this.canvas.width, this.canvas.height)
            this.ctx.beginPath()
            this.ctx.arc(x, y, r, 0, 2 * Math.PI)
            this.ctx.fill()
            this.ctx.stroke()
        }
    }
}

export class Triangle extends CanvasTool {
    public static name = 'Triangle'

    public constructor(canvas: HTMLCanvasElement) {
        super(canvas)
    }

    #startX: number = 0
    #startY: number = 0

    #saved: string = ''

    protected mouseDownHandler() {
        this.#saved = this.canvas.toDataURL()
        this.#startX = this.mousePosition.x
        this.#startY = this.mousePosition.y
        this.ctx.beginPath()
    }

    protected mouseMoveHandler() {
        if (this.mouseDown) {
            const width = this.mousePosition.x - this.#startX
            const height = this.mousePosition.y - this.#startY
            this.draw(this.#startX, this.#startY, width, height)
        }
    }

    protected mouseLeaveHandler() {}

    protected mouseUpHandler() {}

    protected draw(x: number, y: number, w: number, h: number) {
        const image = new Image()
        image.src = this.#saved
        image.onload = () => {
            this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height)
            this.ctx.drawImage(image, 0, 0, this.canvas.width, this.canvas.height)
            this.ctx.beginPath()
            this.ctx.rect(x, y, w, h)
            this.ctx.fill()
            this.ctx.stroke()
        }
    }
}

export class Square extends CanvasTool {
    public static name = 'Square'

    public constructor(canvas: HTMLCanvasElement) {
        super(canvas)
    }

    #startX: number = 0
    #startY: number = 0

    #saved: string = ''

    protected mouseDownHandler() {
        this.#saved = this.canvas.toDataURL()

        this.ctx.beginPath()
        this.#startX = this.mousePosition.x
        this.#startY = this.mousePosition.y
    }

    protected mouseMoveHandler() {
        if (this.mouseDown) {
            const width = this.mousePosition.x - this.#startX
            const height = this.mousePosition.y - this.#startY
            this.draw(this.#startX, this.#startY, width, height)
        }
    }

    protected mouseLeaveHandler() {}

    protected mouseUpHandler() {}

    protected draw(x: number, y: number, w: number, h: number) {
        const image = new Image()
        image.src = this.#saved
        image.onload = () => {
            this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height)
            this.ctx.drawImage(image, 0, 0, this.canvas.width, this.canvas.height)
            this.ctx.beginPath()
            this.ctx.rect(x, y, w, h)
            this.ctx.fill()
            this.ctx.stroke()
        }
    }

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
