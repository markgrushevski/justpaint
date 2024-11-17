export async function createImage(src: string): Promise<HTMLImageElement> {
    const image = document.createElement('img')

    return new Promise((resolve) => {
        image.onload = () => {
            resolve(image)
        }
        image.src = src
    })
}
