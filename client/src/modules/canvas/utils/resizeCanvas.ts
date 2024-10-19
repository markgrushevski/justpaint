/**
 * Изменяет размер канваса на размер родительского элемента.
 * @param {HTMLCanvasElement} canvas Элемент канваса.
 * @param {number} [width]
 * @param {number} [height]
 * @throws {Error} Если не найден контекст канваса.
 * @return {Promise<boolean>} Промис, который резолвится после успешной перерисовки в новом размере.
 * */
export async function resizeCanvas(canvas: HTMLCanvasElement, width?: number, height?: number): Promise<boolean> {
    return new Promise<boolean>((resolve, reject) => {
        const _width = Math.round(canvas.parentElement!.clientWidth)
        const _height = Math.round(canvas.parentElement!.clientHeight)

        const ctx = canvas.getContext('2d')
        if (ctx) {
            const image = new Image()
            image.onload = () => {
                ctx.clearRect(0, 0, ctx.canvas.width, ctx.canvas.height)

                ctx.canvas.width = width ?? _width
                ctx.canvas.height = height ?? _height

                ctx.drawImage(image, 0, 0, ctx.canvas.width, ctx.canvas.height)

                resolve(true)
            }
            image.src = ctx.canvas.toDataURL('image/png', 1)
        } else {
            reject(new Error('CanvasRenderingContext2D not found'))
        }
    })
}
