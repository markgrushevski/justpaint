class Tool {
    constructor(canvas) {
        this.canvas = canvas;
        this.ctx = canvas.getContext('2d');
        this.destroy();
    }

    destroy() {
        this.canvas.onmousedown = null;
        this.canvas.onmousemove = null;
        this.canvas.onmouseup = null;
    }
}

export class Pen extends Tool {
    constructor(canvas) {
        super(canvas);
        this.listen();
    }

    listen() {
        this.canvas.onmousedown = this.mouseDownHandler.bind(this);
        this.canvas.onmousemove = this.mouseMoveHandler.bind(this);
        this.canvas.onmouseup = this.mouseUpHandler.bind(this);
    }

    mouseDownHandler(ev) {
        this.mouseDown = true;
        this.ctx.beginPath();
        this.ctx.moveTo(e.pageX - e.target.offsetLeft, e.pageY - e.target.offsetTop);
    }

    mouseMoveHandler(ev) {
        if (this.mouseDown) {
            this.draw(e.pageX - e.target.offsetLeft, e.pageY - e.target.offsetTop);
        }
    }

    mouseUpHandler(ev) {
        this.mouseDown = false;
    }

    draw(x, y) {
        this.ctx.lineTo(x, y);
        this.ctx.stroke();
    }
}
