import {
    mdiBrush,
    mdiBrushOutline,
    mdiCheck,
    mdiCircleOutline,
    mdiCloudDownloadOutline,
    mdiContentCopy,
    mdiContentSaveOutline,
    mdiEraser,
    mdiRedo,
    mdiSquareOutline,
    mdiTriangleOutline,
    mdiUndo,
    mdiVectorLine,
    mdiWindowClose
} from '@mdi/js'

export const icons = {
    draw: {
        Eraser: mdiEraser,
        Pen: mdiBrush,
        Line: mdiVectorLine,
        Circle: mdiCircleOutline,
        Triangle: mdiTriangleOutline,
        Square: mdiSquareOutline
    },
    work: {
        undo: mdiUndo,
        redo: mdiRedo,
        save: mdiContentSaveOutline,
        load: mdiCloudDownloadOutline,
        copy: mdiContentCopy
    },
    info: {
        success: mdiCheck,
        error: mdiWindowClose
    }
}
