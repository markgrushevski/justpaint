export abstract class CanvasTool {
    protected abstract mouseDownHandler(ev: MouseEvent): void;
    protected abstract mouseMoveHandler(ev: MouseEvent): void;
    protected abstract mouseLeaveHandler(ev: MouseEvent): void;
    protected abstract mouseUpHandler(ev: MouseEvent): void;
    protected abstract draw(...args: unknown[]): void;

    protected constructor(canvas: HTMLCanvasElement) {
        this.canvas = canvas;

        this.ctx = canvas.getContext('2d') as CanvasRenderingContext2D;
        if (!this.ctx) throw new Error('CanvasRenderingContext2D not found');
        this.ctx.lineCap = 'round';
        this.ctx.lineJoin = 'round';
        this.ctx.imageSmoothingEnabled = true;
        this.ctx.imageSmoothingQuality = 'high';

        this.mouseDown = false;

        this.destroy();
        this.listen();
    }

    #mousePosition: { x: number; y: number } = { x: 0, y: 0 };
    protected canvas: HTMLCanvasElement;
    protected ctx: CanvasRenderingContext2D;
    protected mouseDown: boolean;

    public get mousePosition(): { x: number; y: number } {
        return this.#mousePosition;
    }

    public get strokeColor(): typeof this.ctx.strokeStyle {
        return this.ctx.strokeStyle;
    }

    public get fillColor(): typeof this.ctx.fillStyle {
        return this.ctx.fillStyle;
    }

    public get lineWeight() {
        return this.ctx.lineWidth;
    }

    public set strokeColor(color: string) {
        this.ctx.strokeStyle = color;
    }

    public set fillColor(color: string) {
        this.ctx.fillStyle = color;
    }

    public set lineWeight(number: number) {
        this.ctx.lineWidth = number;
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

        this.canvas.addEventListener('mousedown', (ev) => {
            console.log('Down');
        });

        this.canvas.addEventListener('mousemove', (ev) => {
            this.#mousePosition.x = ev.pageX - ev.target?.offsetLeft;
            this.#mousePosition.y = ev.pageY - ev.target?.offsetTop;
            console.log('Move');
        });

        this.canvas.addEventListener('mouseleave', (ev) => {
            console.log('Leave');
        });

        this.canvas.addEventListener('mouseup', (ev) => {
            console.log('Up');
        });

        this.canvas.addEventListener('touchmove', (ev) => {
            console.log('touch Move');
        });

        this.canvas.addEventListener('touchstart', (ev) => {
            console.log('touch Start');
        });

        this.canvas.addEventListener('touchend', (ev) => {
            console.log('touch End');
        });

        this.canvas.addEventListener('touchcancel', (ev) => {
            console.log('touch Cancel');
        });
    }
}

export class Eraser extends CanvasTool {
    public constructor(canvas: HTMLCanvasElement) {
        super(canvas);
    }

    protected mouseDownHandler(ev: MouseEvent) {
        this.mouseDown = true;
        this.ctx.beginPath();
        this.ctx.moveTo(this.mousePosition.x, this.mousePosition.y);
    }

    protected mouseMoveHandler(ev: MouseEvent) {
        if (this.mouseDown) {
            this.draw(this.mousePosition.x, this.mousePosition.y);
        }
    }

    protected mouseLeaveHandler(ev: MouseEvent) {
        this.mouseDown = false;
    }

    protected mouseUpHandler(ev: MouseEvent) {
        this.mouseDown = false;
    }

    protected draw(x: number, y: number) {
        this.ctx.lineTo(x, y);
        this.ctx.stroke();
        this.ctx.strokeStyle = '#ffffff';
    }
}

export class Pen extends CanvasTool {
    public constructor(canvas: HTMLCanvasElement) {
        super(canvas);
    }

    protected mouseDownHandler(ev: MouseEvent) {
        this.mouseDown = true;
        this.ctx.beginPath();
        this.ctx.moveTo(this.mousePosition.x, this.mousePosition.y);
    }

    protected mouseMoveHandler(ev: MouseEvent) {
        if (this.mouseDown) {
            this.draw(this.mousePosition.x, this.mousePosition.y);
        }
    }

    protected mouseLeaveHandler(ev: MouseEvent) {
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

export class Line extends CanvasTool {
    public constructor(canvas: HTMLCanvasElement) {
        super(canvas);
    }

    #startX: number = 0;
    #startY: number = 0;

    #saved: string = '';

    protected mouseDownHandler(ev: MouseEvent) {
        this.mouseDown = true;
        this.#saved = this.canvas.toDataURL();
        this.#startX = this.mousePosition.x;
        this.#startY = this.mousePosition.y;
        this.ctx.beginPath();
        this.ctx.moveTo(this.#startX, this.#startY);
    }

    protected mouseMoveHandler(ev: MouseEvent) {
        if (this.mouseDown) {
            this.draw(this.mousePosition.x, this.mousePosition.y);
        }
    }

    protected mouseLeaveHandler(ev: MouseEvent) {
        this.mouseDown = false;
    }

    protected mouseUpHandler(ev: MouseEvent) {
        this.mouseDown = false;
    }

    protected draw(x: number, y: number) {
        const image = new Image();
        image.src = this.#saved;
        image.onload = () => {
            this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
            this.ctx.drawImage(image, 0, 0, this.canvas.width, this.canvas.height);
            this.ctx.beginPath();
            this.ctx.moveTo(this.#startX, this.#startY);
            this.ctx.lineTo(x, y);
            this.ctx.stroke();
        };
    }
}

export class Circle extends CanvasTool {
    public constructor(canvas: HTMLCanvasElement) {
        super(canvas);
    }

    #startX: number = 0;
    #startY: number = 0;

    #saved: string = '';

    protected mouseDownHandler(ev: MouseEvent) {
        this.mouseDown = true;
        this.#saved = this.canvas.toDataURL();
        this.#startX = this.mousePosition.x;
        this.#startY = this.mousePosition.y;
        this.ctx.beginPath();
    }

    protected mouseMoveHandler(ev: MouseEvent) {
        if (this.mouseDown) {
            const width = this.mousePosition.x - this.#startX;
            const height = this.mousePosition.y - this.#startY;
            const r = Math.sqrt(width ** 2 + height ** 2);
            this.draw(this.#startX, this.#startY, r);
        }
    }

    protected mouseLeaveHandler(ev: MouseEvent) {
        this.mouseDown = false;
    }

    protected mouseUpHandler(ev: MouseEvent) {
        this.mouseDown = false;
    }

    protected draw(x: number, y: number, r: number) {
        const image = new Image();
        image.src = this.#saved;
        image.onload = () => {
            this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
            this.ctx.drawImage(image, 0, 0, this.canvas.width, this.canvas.height);
            this.ctx.beginPath();
            this.ctx.arc(x, y, r, 0, 2 * Math.PI);
            this.ctx.fill();
            this.ctx.stroke();
        };
    }
}

export class Triangle extends CanvasTool {
    public constructor(canvas: HTMLCanvasElement) {
        super(canvas);
    }

    #startX: number = 0;
    #startY: number = 0;

    #saved: string = '';

    protected mouseDownHandler(ev: MouseEvent) {
        this.mouseDown = true;
        this.#saved = this.canvas.toDataURL();
        this.#startX = this.mousePosition.x;
        this.#startY = this.mousePosition.y;
        this.ctx.beginPath();
    }

    protected mouseMoveHandler(ev: MouseEvent) {
        if (this.mouseDown) {
            const width = this.mousePosition.x - this.#startX;
            const height = this.mousePosition.y - this.#startY;
            this.draw(this.#startX, this.#startY, width, height);
        }
    }

    protected mouseLeaveHandler(ev: MouseEvent) {
        this.mouseDown = false;
    }

    protected mouseUpHandler(ev: MouseEvent) {
        this.mouseDown = false;
    }

    protected draw(x: number, y: number, w: number, h: number) {
        const image = new Image();
        image.src = this.#saved;
        image.onload = () => {
            this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
            this.ctx.drawImage(image, 0, 0, this.canvas.width, this.canvas.height);
            this.ctx.beginPath();
            this.ctx.rect(x, y, w, h);
            this.ctx.fill();
            this.ctx.stroke();
        };
    }
}

export class Square extends CanvasTool {
    public constructor(canvas: HTMLCanvasElement) {
        super(canvas);
    }

    #startX: number = 0;
    #startY: number = 0;

    #saved: string = '';

    protected mouseDownHandler(ev: MouseEvent) {
        this.#saved = this.canvas.toDataURL();
        this.mouseDown = true;
        this.ctx.beginPath();
        this.#startX = this.mousePosition.x;
        this.#startY = this.mousePosition.y;
    }

    protected mouseMoveHandler(ev: MouseEvent) {
        if (this.mouseDown) {
            const width = this.mousePosition.x - this.#startX;
            const height = this.mousePosition.y - this.#startY;
            this.draw(this.#startX, this.#startY, width, height);
        }
    }

    protected mouseLeaveHandler(ev: MouseEvent) {
        this.mouseDown = false;
    }

    protected mouseUpHandler(ev: MouseEvent) {
        this.mouseDown = false;
    }

    protected draw(x: number, y: number, w: number, h: number) {
        const image = new Image();
        image.src = this.#saved;
        image.onload = () => {
            this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
            this.ctx.drawImage(image, 0, 0, this.canvas.width, this.canvas.height);
            this.ctx.beginPath();
            this.ctx.rect(x, y, w, h);
            this.ctx.fill();
            this.ctx.stroke();
        };
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
