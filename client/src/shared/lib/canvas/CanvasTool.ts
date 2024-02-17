export abstract class CanvasTool {
    protected abstract mouseDownHandler(ev: MouseEvent): void;
    protected abstract mouseMoveHandler(ev: MouseEvent): void;
    protected abstract mouseLeaveHandler(ev: MouseEvent): void;
    protected abstract mouseUpHandler(ev: MouseEvent): void;
    protected abstract draw(...args: unknown[]): void;

    protected constructor(canvas: HTMLCanvasElement) {
        this.canvas = canvas;
        this.ctx = canvas.getContext('2d');
        this.destroy();
        this.listen();
    }

    protected canvas: HTMLCanvasElement;
    protected ctx: CanvasRenderingContext2D | null;
    protected mouseDown: boolean;
    protected mousePosition: { x: number; y: number };

    protected getMousePosition(ev: MouseEvent): { x: number; y: number } {
        return {
            x: ev.pageX - ev.target?.offsetLeft,
            y: ev.pageY - ev.target?.offsetTop
        };
    }

    public destroy() {
        this.canvas.onmousedown = null;
        this.canvas.onmousemove = null;
        this.canvas.onmouseleave = null;
        this.canvas.onmouseup = null;
    }

    public listen() {
        this.canvas.onmousedown = this.mouseDownHandler.bind(this);
        this.canvas.onmousemove = this.mouseMoveHandler.bind(this);
        this.canvas.onmouseleave = this.mouseLeaveHandler.bind(this);
        this.canvas.onmouseup = this.mouseUpHandler.bind(this);

        this.canvas.addEventListener('mousemove');
    }
}

export class Eraser extends CanvasTool {
    public constructor(canvas: HTMLCanvasElement) {
        super(canvas);
    }

    protected mouseDownHandler(ev: MouseEvent) {
        const mousePosition = this.getMousePosition(ev);
        this.mouseDown = true;
        this.ctx?.beginPath();
        this.ctx?.moveTo(mousePosition.x, mousePosition.y);
    }

    protected mouseMoveHandler(ev: MouseEvent) {
        if (this.mouseDown) {
            const mousePosition = this.getMousePosition(ev);
            this.draw(mousePosition.x, mousePosition.y);
        }
    }

    protected mouseLeaveHandler(ev: MouseEvent) {
        this.mouseDown = false;
    }

    protected mouseUpHandler(ev: MouseEvent) {
        this.mouseDown = false;
    }

    protected draw(x: number, y: number) {
        this.ctx?.lineTo(x, y);
        this.ctx?.stroke();
    }
}

export class Pen extends CanvasTool {
    public constructor(canvas: HTMLCanvasElement) {
        super(canvas);
    }

    protected mouseDownHandler(ev: MouseEvent) {
        const mousePosition = this.getMousePosition(ev);
        this.mouseDown = true;
        this.ctx?.beginPath();
        this.ctx?.moveTo(mousePosition.x, mousePosition.y);
    }

    protected mouseMoveHandler(ev: MouseEvent) {
        if (this.mouseDown) {
            const mousePosition = this.getMousePosition(ev);
            this.draw(mousePosition.x, mousePosition.y);
        }
    }

    protected mouseLeaveHandler(ev: MouseEvent) {
        this.mouseDown = false;
    }

    protected mouseUpHandler(ev: MouseEvent) {
        this.mouseDown = false;
    }

    protected draw(x: number, y: number) {
        this.ctx?.lineTo(x, y);
        this.ctx?.stroke();
    }
}

export class Square extends CanvasTool {
    public constructor(canvas: HTMLCanvasElement) {
        super(canvas);
    }

    #startX: number = 0;
    #startY: number = 0;

    protected mouseDownHandler(ev: MouseEvent) {
        const mousePosition = this.getMousePosition(ev);
        this.mouseDown = true;
        this.ctx?.beginPath();
        this.#startX = mousePosition.x;
        this.#startY = mousePosition.y;
    }

    protected mouseMoveHandler(ev: MouseEvent) {
        if (this.mouseDown) {
            const mousePosition = this.getMousePosition(ev);
            const currentX = mousePosition.x;
            const currentY = mousePosition.y;
            const width = currentX - this.#startX;
            const height = currentY - this.#startY;
            this.draw(mousePosition.x, mousePosition.y, width, height);
        }
    }

    protected mouseLeaveHandler(ev: MouseEvent) {
        this.mouseDown = false;
    }

    protected mouseUpHandler(ev: MouseEvent) {
        this.mouseDown = false;
    }

    protected draw(x: number, y: number, w: number, h: number) {
        const mousePosition = this.getMousePosition(ev);
    }
}
