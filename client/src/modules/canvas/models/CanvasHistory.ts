import { CanvasHistoryModel } from './CanvasModel.ts'

export type CanvasHistoryStep = {
    canvasWidth: number
    canvasHeight: number
    canvasDataURL: string
}

export class CanvasHistory extends CanvasHistoryModel {
    public static name = 'CanvasWork'

    public constructor(canvas: HTMLCanvasElement) {
        super(canvas)

        this.historyIndex = 0
        this.history = [
            {
                canvasWidth: canvas.clientWidth,
                canvasHeight: canvas.clientHeight,
                canvasDataURL: this.canvasDataURL
            }
        ]
    }

    private historyIndex: number
    public readonly history: CanvasHistoryStep[]

    public step(step: CanvasHistoryStep) {
        const currentStep = this.history[this.historyIndex]
        if (currentStep.canvasDataURL !== step.canvasDataURL) {
            this.history.length = this.historyIndex + 1
            this.history.push(step)
            this.historyIndex += 1
        }
    }

    public stepBack() {
        const prevStep = this.history[this.historyIndex - 1]
        if (prevStep) {
            this.historyIndex -= 1
            return prevStep
        }
    }

    public stepForward() {
        const nextStep = this.history[this.historyIndex + 1]
        if (nextStep) {
            this.historyIndex += 1
            return nextStep
        }
    }

    public save() {}

    public load() {}

    public get hasPrevStep(): boolean {
        return this.historyIndex > 0
    }

    public get hasNextStep(): boolean {
        return this.historyIndex < this.history.length - 1
    }
}
