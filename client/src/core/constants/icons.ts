import {
    mdiBrush,
    mdiBrushOutline,
    mdiCheck,
    mdiCircleOutline,
    mdiCloudDownloadOutline,
    mdiContentCopy,
    mdiContentSaveOutline,
    mdiEraser,
    mdiLogout,
    mdiRedo,
    mdiRename,
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
    art: {
        undo: mdiUndo,
        redo: mdiRedo,
        save: mdiContentSaveOutline,
        load: mdiCloudDownloadOutline,
        copy: mdiContentCopy,
        rename: mdiRename,
        accept: mdiCheck
    },
    info: {
        success: mdiCheck,
        error: mdiWindowClose
    },
    auth: {
        logout: mdiLogout
    }
}
