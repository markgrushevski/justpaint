export abstract class CanvasTool {
    protected abstract mouseDownHandler(ev: MouseEvent): void;
    protected abstract mouseMoveHandler(ev: MouseEvent): void;
    protected abstract mouseOutHandler(ev: MouseEvent): void;
    protected abstract mouseUpHandler(ev: MouseEvent): void;
    protected abstract draw(...args: unknown[]): void;

    protected constructor(canvas: HTMLCanvasElement) {
        this.canvas = canvas;
        this.ctx = canvas.getContext('2d');
        this.mouseDown = false;
        this.destroy();
        this.listen();
    }

    protected canvas: HTMLCanvasElement;
    protected ctx: CanvasRenderingContext2D | null;
    protected mouseDown: boolean;

    public destroy() {
        this.canvas.onmousedown = null;
        this.canvas.onmousemove = null;
        this.canvas.onmouseout = null;
        this.canvas.onmouseup = null;
    }

    public listen() {
        this.canvas.onmousedown = this.mouseDownHandler.bind(this);
        this.canvas.onmousemove = this.mouseMoveHandler.bind(this);
        this.canvas.onmouseout = this.mouseOutHandler.bind(this);
        this.canvas.onmouseup = this.mouseUpHandler.bind(this);
    }
}

export class Pen extends CanvasTool {
    public constructor(canvas: HTMLCanvasElement) {
        super(canvas);
    }

    protected mouseDownHandler(ev: MouseEvent) {
        this.mouseDown = true;
        this.ctx.beginPath();
        this.ctx.moveTo(ev.pageX - ev.target.offsetLeft, ev.pageY - ev.target.offsetTop);
    }

    protected mouseMoveHandler(ev: MouseEvent) {
        if (this.mouseDown) {
            this.draw(ev.pageX - ev.target.offsetLeft, ev.pageY - ev.target.offsetTop);
        }
    }

    protected mouseOutHandler(ev: MouseEvent) {
        this.mouseDown = false;
    }

    protected mouseUpHandler(ev: MouseEvent) {
        this.mouseDown = false;
    }

    protected draw(x: number, y: number) {
        this.ctx.lineTo(x, y);
        this.ctx.stroke();
    }
}

export class Eraser extends CanvasTool {
    constructor(canvas: HTMLCanvasElement) {
        super(canvas);
    }

    protected mouseDownHandler(ev: MouseEvent) {
        this.mouseDown = true;
        this.ctx.beginPath();
        this.ctx.moveTo(ev.pageX - ev.target.offsetLeft, ev.pageY - ev.target.offsetTop);
    }

    protected mouseMoveHandler(ev: MouseEvent) {
        if (this.mouseDown) {
            this.draw(ev.pageX - ev.target.offsetLeft, ev.pageY - ev.target.offsetTop);
        }
    }

    protected mouseOutHandler(ev: MouseEvent) {
        this.mouseDown = false;
    }

    protected mouseUpHandler(ev: MouseEvent) {
        this.mouseDown = false;
    }

    protected draw(x: number, y: number) {
        this.ctx.lineTo(x, y);
        this.ctx.stroke();
    }
}
