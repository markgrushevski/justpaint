import { CanvasHistoryModel } from './models'

export class CanvasHistory extends CanvasHistoryModel {
    public static name = 'CanvasWork'

    public constructor(canvas: HTMLCanvasElement) {
        super(canvas)
    }

    private actionHistoryList = []

    private loadStateToCanvas() {}

    private makeActionHistoryStep() {}

    public undo() {}

    public redo() {}

    public save() {}

    public load() {}
}
