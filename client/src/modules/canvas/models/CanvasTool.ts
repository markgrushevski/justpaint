import type { CanvasHistory } from './CanvasHistory'
import { CanvasToolModel } from './CanvasModel.ts'

export type ToolClass = typeof Pen | typeof Eraser | typeof Line | typeof Circle | typeof Triangle | typeof Square
export type ToolName = ToolClass['name']

type DrawHandlerEvent = MouseEvent | Touch
type EventHandlersMap = { [P in keyof GlobalEventHandlersEventMap]?: (ev: GlobalEventHandlersEventMap[P]) => void }

abstract class CanvasTool extends CanvasToolModel {
    protected static name = 'CanvasTool'

    protected abstract drawStartHandler(ev: DrawHandlerEvent): void
    protected abstract drawHandler(ev: DrawHandlerEvent): void
    protected abstract drawEndHandler(ev: DrawHandlerEvent): void

    protected constructor(canvas: HTMLCanvasElement, canvasHistory: CanvasHistory) {
        super(canvas)

        this.canvasHistory = canvasHistory
        this.ctx.lineCap = 'round'
        this.ctx.lineJoin = 'round'
        this.ctx.imageSmoothingEnabled = true
        this.ctx.imageSmoothingQuality = 'high'

        this.destroy()
        this.listen()

        if (import.meta.env.DEV) {
            this.ctx.strokeStyle = '#8000ff'
            this.ctx.fillStyle = '#8080ff'
            this.ctx.lineWidth = 2
        }
    }

    // public

    // protected

    protected canvasHistory: CanvasHistory

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
        },
        end: {
            x: 0,
            y: 0,
            width: 0,
            height: 0,
            canvasDataURL: ''
        }
    }

    protected eventHandlersMap: EventHandlersMap = {
        mousedown: this.handleMouseDown,
        mousemove: this.handleMouseMove,
        mouseleave: this.handleMouseLeave,
        mouseup: this.handleMouseUp,
        touchstart: this.handleTouchStart,
        touchmove: this.handleTouchMove,
        touchend: this.handleTouchEnd,
        touchcancel: this.handleTouchCancel
        /*  wheel: this.handleWheel */
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
        this.handleStart(ev)
    }

    private handleMouseMove(ev: MouseEvent) {
        this.handleMove(ev)
    }

    private handleMouseLeave(ev: MouseEvent) {
        this.handleEnd(ev)
    }

    private handleMouseUp(ev: MouseEvent) {
        this.handleEnd(ev)
    }

    private handleTouchStart(ev: TouchEvent) {
        const touchEv = ev.touches[0]
        this.handleStart(touchEv)
    }

    private handleTouchMove(ev: TouchEvent) {
        const touchEv = ev.touches[0]
        this.handleMove(touchEv)
    }

    private handleTouchEnd(ev: TouchEvent) {
        const touchEv = ev.touches[0]
        this.handleEnd(touchEv)
    }

    private handleTouchCancel(ev: TouchEvent) {
        const touchEv = ev.touches[0]
        this.handleEnd(touchEv)
    }

    // common

    private handleStart(ev: DrawHandlerEvent) {
        this.mouseDown = true
        this.setStartDrawData()
        this.drawStartHandler(ev)
    }

    private handleMove(ev: DrawHandlerEvent) {
        this.setCurrentDrawData(ev)

        if (this.mouseDown) {
            if (this.isFigure) {
                this.loadStateToCanvas(this.drawData.start.canvasDataURL).then((loaded) => {
                    if (loaded) this.drawHandler(ev)
                })
            } else {
                this.drawHandler(ev)
            }
        }
    }

    private handleEnd(ev: DrawHandlerEvent) {
        if (this.mouseDown) {
            this.mouseDown = false
            this.drawEndHandler(ev)
            this.setEndDrawData()
            this.canvasHistory.step({
                canvasWidth: this.canvas.clientWidth,
                canvasHeight: this.canvas.clientHeight,
                canvasDataURL: this.canvasDataURL
            })
        }
    }

    private setStartDrawData() {
        this.drawData.start.canvasDataURL = this.canvasDataURL

        this.drawData.start.x = this.drawData.current.x
        this.drawData.start.y = this.drawData.current.y
    }

    private setCurrentDrawData(ev: DrawHandlerEvent) {
        this.drawData.current.x = ev.pageX - (ev.target as HTMLCanvasElement).offsetLeft
        this.drawData.current.y = ev.pageY - (ev.target as HTMLCanvasElement).offsetTop

        this.drawData.current.width = this.drawData.current.x - this.drawData.start.x
        this.drawData.current.height = this.drawData.current.y - this.drawData.start.y
    }

    private setEndDrawData() {
        this.drawData.end = {
            ...this.drawData.current,
            canvasDataURL: this.canvasDataURL
        }
    }

    public loadStateToCanvas(canvasDataURL: string, force?: boolean): Promise<boolean> {
        return new Promise((resolve) => {
            if (canvasDataURL) {
                const image = new Image()
                image.onload = () => {
                    if (this.mouseDown || force) {
                        this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height)
                        this.ctx.drawImage(image, 0, 0, this.canvas.width, this.canvas.height)
                        resolve(true)
                    } else {
                        resolve(false)
                    }
                }
                image.src = canvasDataURL
            } else {
                this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height)
            }
        })
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

    public constructor(canvas: HTMLCanvasElement, canvasHistory: CanvasHistory) {
        super(canvas, canvasHistory)
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

    public constructor(canvas: HTMLCanvasElement, canvasHistory: CanvasHistory) {
        super(canvas, canvasHistory)
        this.isFigure = false
    }

    protected drawStartHandler() {
        this.ctx.save()
        this.ctx.globalCompositeOperation = 'destination-out'
        this.ctx.beginPath()
        this.ctx.moveTo(this.drawData.current.x, this.drawData.current.y)
    }

    protected drawHandler() {
        this.ctx.lineTo(this.drawData.current.x, this.drawData.current.y)
        this.ctx.stroke()
    }

    protected drawEndHandler() {
        this.ctx.restore()
    }
}

export class Line extends CanvasTool {
    public static readonly name = 'Line'

    public constructor(canvas: HTMLCanvasElement, canvasHistory: CanvasHistory) {
        super(canvas, canvasHistory)
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

    public constructor(canvas: HTMLCanvasElement, canvasHistory: CanvasHistory) {
        super(canvas, canvasHistory)
    }

    protected drawStartHandler() {
        this.ctx.beginPath()
    }

    protected drawHandler() {
        const centerX = this.drawData.start.x + this.drawData.current.width / 2
        const centerY = this.drawData.start.y + this.drawData.current.height / 2

        const radiusX = Math.abs(this.drawData.current.width) / Math.sqrt(2)
        const radiusY = Math.abs(this.drawData.current.height) / Math.sqrt(2)

        this.ctx.beginPath()
        this.ctx.ellipse(centerX, centerY, radiusX, radiusY, 0, 0, Math.PI * 2)
        this.ctx.fill()
        this.ctx.stroke()
    }

    protected drawEndHandler() {}
}

export class Triangle extends CanvasTool {
    public static readonly name = 'Triangle'

    public constructor(canvas: HTMLCanvasElement, canvasHistory: CanvasHistory) {
        super(canvas, canvasHistory)
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

    public constructor(canvas: HTMLCanvasElement, canvasHistory: CanvasHistory) {
        super(canvas, canvasHistory)
    }

    protected drawStartHandler() {
        console.log('drawStartHandler')
        this.ctx.beginPath()
    }

    protected drawHandler() {
        console.log('drawHandler')
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

    protected drawEndHandler() {
        console.log('drawEndHandler')
    }
}
