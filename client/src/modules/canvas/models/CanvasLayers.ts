import { CanvasLayersModel } from './CanvasModel.ts'

export class CanvasLayers extends CanvasLayersModel {
    public static name = 'CanvasLayers'

    public constructor(canvas: HTMLCanvasElement) {
        super(canvas)
    }
}
