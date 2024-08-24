import { CanvasToolModel } from './models'

export type Tool = Pen | Eraser | Line | Circle | Triangle | Square

type DrawHandlerEvent = MouseEvent | Touch

type EventHandlersMap = { [P in keyof GlobalEventHandlersEventMap]?: (ev: GlobalEventHandlersEventMap[P]) => void }

abstract class CanvasTool extends CanvasToolModel {
    protected static name = 'CanvasTool'

    protected abstract drawStartHandler(ev: DrawHandlerEvent): void
    protected abstract drawHandler(ev: DrawHandlerEvent): void
    protected abstract drawEndHandler(ev: DrawHandlerEvent): void

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

    // public

    // protected

    protected isFigure = true
    protected mouseDown = false

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

    protected eventHandlersMap: EventHandlersMap = {
        mousedown: this.handleMouseDown,
        mousemove: this.handleMouseMove,
        mouseleave: this.handleMouseLeave,
        mouseup: this.handleMouseUp
        /* touchstart: this.handleTouchStart,
        touchmove: this.handleTouchMove,
        touchend: this.handleTouchEnd,
        touchcancel: this.handleTouchCancel,
        wheel: this.handleWheel */
    }

    // private

    private listen() {
        Object.entries(this.eventHandlersMap).forEach(([eventName, listener]) => {
            // @ts-expect-error expect that keys must be in canvas element
            this.canvas[`on${eventName}`] = listener.bind(this)
        })
    }

    private destroy() {
        Object.entries(this.eventHandlersMap).forEach(([eventName]) => {
            // @ts-expect-error expect that keys must be in canvas element
            this.canvas[`on${eventName}`] = null
        })
    }

    // handlers

    private handleMouseDown(ev: MouseEvent) {
        this.mouseDown = true
        this.setStartDrawData()
        this.drawStartHandler(ev)
    }

    private handleMouseMove(ev: MouseEvent) {
        this.setCurrentDrawData(ev)
        this.handleMove(ev)
    }

    private handleMouseLeave(ev: MouseEvent) {
        this.mouseDown = false
        this.drawEndHandler(ev)
    }

    private handleMouseUp(ev: MouseEvent) {
        this.mouseDown = false
        this.drawEndHandler(ev)
    }

    /* private handleTouchMove(ev: TouchEvent) {
        const touchEv = ev.touches[0]
        this.setCurrentDrawData(touchEv)
        this.handleMove(touchEv)
    } */

    // common

    private handleMove(ev: DrawHandlerEvent) {
        if (this.mouseDown) {
            if (this.isFigure) {
                const image = new Image()
                image.src = this.drawData.start.canvasDataURL
                image.onload = () => {
                    this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height)
                    this.ctx.drawImage(image, 0, 0, this.canvas.width, this.canvas.height)
                    this.drawHandler(ev)
                }
            } else {
                this.drawHandler(ev)
            }
        }
    }

    private setStartDrawData() {
        this.drawData.start.canvasDataURL = this.canvas.toDataURL()

        this.drawData.start.x = this.drawData.current.x
        this.drawData.start.y = this.drawData.current.y
    }

    private setCurrentDrawData(ev: DrawHandlerEvent) {
        this.drawData.current.x = ev.pageX - (ev.target as HTMLCanvasElement).offsetLeft
        this.drawData.current.y = ev.pageY - (ev.target as HTMLCanvasElement).offsetTop

        this.drawData.current.width = this.drawData.current.x - this.drawData.start.x
        this.drawData.current.height = this.drawData.current.y - this.drawData.start.y
    }

    // public getters

    public get strokeColor(): string {
        return this.ctx.strokeStyle as string
    }

    public get fillColor(): string {
        return this.ctx.fillStyle as string
    }

    public get lineWeight(): number {
        return this.ctx.lineWidth
    }

    // public setters

    public set strokeColor(color: string) {
        this.ctx.strokeStyle = color
    }

    public set fillColor(color: string) {
        this.ctx.fillStyle = color
    }

    public set lineWeight(number: number) {
        this.ctx.lineWidth = number
    }
}

export class Pen extends CanvasTool {
    public static readonly name = 'Pen'

    public constructor(canvas: HTMLCanvasElement) {
        super(canvas)
        this.isFigure = false
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
        this.isFigure = false
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
        this.ctx.beginPath()
        this.ctx.moveTo(this.drawData.start.x, this.drawData.start.y)
        this.ctx.lineTo(this.drawData.current.x, this.drawData.current.y)
        this.ctx.stroke()
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

        this.ctx.beginPath()
        this.ctx.ellipse(centerX, centerY, radiusX, radiusY, 0, 0, Math.PI * 2)
        this.ctx.fill()
        this.ctx.stroke()
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
        this.ctx.beginPath()
        this.ctx.rect(
            this.drawData.start.x,
            this.drawData.start.y,
            this.drawData.current.width,
            this.drawData.current.height
        )
        this.ctx.fill()
        this.ctx.stroke()
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
        this.ctx.beginPath()
        this.ctx.rect(
            this.drawData.start.x,
            this.drawData.start.y,
            this.drawData.current.width,
            this.drawData.current.height
        )
        this.ctx.fill()
        this.ctx.stroke()
    }

    protected drawEndHandler() {}
}
