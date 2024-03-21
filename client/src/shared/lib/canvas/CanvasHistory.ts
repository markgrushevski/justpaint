import { CanvasHistoryModel } from './models'

export class CanvasHistory extends CanvasHistoryModel {
    public static name = 'CanvasWork'

    public constructor(canvas: HTMLCanvasElement) {
        super(canvas)
    }

    private actionHistoryList = []

    private loadStateToCanvas() {}

    private makeActionHistoryStep() {}

    public historyHandlersMap = {
        undo: this.handleUndo,
        redo: this.handleRedo,
        save: this.handleSave,
        load: this.handleLoad
    }

    public handleUndo(ev: UIEvent) {}

    public handleRedo(ev: UIEvent) {}

    public handleSave(ev: UIEvent) {}

    public handleLoad(ev: UIEvent) {}
}
