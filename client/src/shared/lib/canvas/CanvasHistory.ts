import { CanvasHistoryModel } from './models';

export class CanvasHistory extends CanvasHistoryModel {
    public static name = 'CanvasWork';

    public constructor(canvas: HTMLCanvasElement) {
        super(canvas);

        this.destroy();
        this.listen();
    }

    private listen() {
        this.canvas.addEventListener('click', () => {});
    }

    public destroy() {}

    private canvasHistoryList = [];

    private loadStateToCanvas() {}

    private makeCanvasHistoryStep() {}

    public eventHandlersMap = {
        undo: this.handleUndo,
        redo: this.handleRedo,
        save: this.handleSave,
        load: this.handleLoad
    };

    public handleUndo(ev: UIEvent) {}

    public handleRedo(ev: UIEvent) {}

    public handleSave(ev: UIEvent) {}

    public handleLoad(ev: UIEvent) {}
}
