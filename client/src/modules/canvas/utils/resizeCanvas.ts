/**
 * Изменяет размер канваса на размер родительского элмента.
 * @param {HTMLCanvasElement} canvas Элемент канваса.
 * @throws {Error} Если не найден контекст канваса.
 * @return {Promise<boolean>} Промис, который резолвится после успешной перерисовки в новом размере.
 * */
export async function resizeCanvas(canvas: HTMLCanvasElement): Promise<boolean> {
    return new Promise<boolean>((resolve, reject) => {
        const width = Math.round(canvas.parentElement!.clientWidth)
        const height = Math.round(canvas.parentElement!.clientHeight)

        const ctx = canvas.getContext('2d')
        if (ctx) {
            const image = new Image()
            image.onload = () => {
                ctx.clearRect(0, 0, ctx.canvas.width, ctx.canvas.height)

                ctx.canvas.width = width
                ctx.canvas.height = height

                ctx.drawImage(image, 0, 0, ctx.canvas.width, ctx.canvas.height)

                resolve(true)
            }
            image.src = ctx.canvas.toDataURL()
        } else {
            reject(new Error('CanvasRenderingContext2D not found'))
        }
    })
}
