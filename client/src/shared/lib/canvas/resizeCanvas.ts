/**
 * Изменяет размер канваса.
 * @param {HTMLCanvasElement} canvas Элемент канваса.
 * @throws {Error} Если не удалось обработать канвас или его контекст.
 * @return {Promise<boolean>} Промис, который резолвится после успешного преобразования канваса в блоб.
 * */
export async function resizeCanvas(canvas: HTMLCanvasElement): Promise<boolean> {
    return new Promise<boolean>((resolve, reject) => {
        const width = Math.round(document.body.clientWidth * 0.7)
        const height = Math.round(document.body.clientWidth * 0.7)

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
