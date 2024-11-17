import { createImage, getCanvasDataURL } from '@modules/canvas'

/**
 * Изменяет размер канваса на размер родительского элемента.
 * @param {HTMLCanvasElement} canvas Элемент канваса.
 * @param {number} [width]
 * @param {number} [height]
 * @throws {Error} Если не найден контекст канваса.
 * @return {Promise<boolean>} Промис, который резолвится после успешной перерисовки в новом размере.
 * */
export async function resizeCanvas(canvas: HTMLCanvasElement, width?: number, height?: number): Promise<boolean> {
    const ctx = canvas.getContext('2d')
    if (ctx) {
        const parentWidth = Math.round(canvas.parentElement!.clientWidth)
        const parentHeight = Math.round(canvas.parentElement!.clientHeight)

        const dataURL = getCanvasDataURL(ctx.canvas)
        const image = await createImage(dataURL)

        ctx.clearRect(0, 0, ctx.canvas.width, ctx.canvas.height)

        ctx.canvas.width = width ?? parentWidth
        ctx.canvas.height = height ?? parentHeight

        ctx.drawImage(image, 0, 0, ctx.canvas.width, ctx.canvas.height)

        return true
    } else {
        throw new Error('CanvasRenderingContext2D not found')
    }
}
