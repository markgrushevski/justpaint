import { Buffer } from 'node:buffer'
import { BadRequestException, Injectable } from '@nestjs/common'
import { InjectRepository } from '@nestjs/typeorm'
import { Repository } from 'typeorm'
import { SaveArtDto } from './dto/arts.dto'
import { ArtEntity } from './entities/art.entity'
import { CompressedLayer, DecompressedLayer } from './types/arts.types'
import { LayersService } from './services/layers.service'

@Injectable()
export class ArtsService {
    constructor(
        @InjectRepository(ArtEntity)
        private readonly artsRepository: Repository<ArtEntity>,
        private readonly layersService: LayersService
    ) {}

    async get(userId: string): Promise<ArtEntity[]> {
        const arts = await this.artsRepository.find({ where: { userId } })

        for (const art of arts) {
            const layers = await this.layersService.get(art.id!)
            art.layers = (await this.decompressLayers(layers ?? [])) ?? []
        }

        return arts
    }

    async save(userId: string, art: SaveArtDto): Promise<ArtEntity> {
        const { layers, ...artEntity } = art

        const savedArt = await this.artsRepository.save({ ...artEntity, userId })

        const compressedLayers = await this.compressLayers(layers)
        if (compressedLayers) {
            compressedLayers.forEach((layer) => {
                layer.artId = savedArt.id!
            })
        } else {
            throw new BadRequestException()
        }

        savedArt.layers = await this.layersService.save(
            compressedLayers as Required<(typeof compressedLayers)[number]>[]
        )

        return savedArt
    }

    async delete(userId: string, artId: string): Promise<boolean> {
        await this.artsRepository.delete({ id: artId, userId })
        return true
    }

    private async compressLayers<T extends { dataURL: string }>(
        layers: DecompressedLayer<T>[]
    ): Promise<CompressedLayer<T>[] | undefined> {
        const compressedLayers: CompressedLayer<T>[] = []

        for (const layer of layers) {
            const dataURL = await this.compressDataURL(layer.dataURL)
            if (!dataURL) return
            compressedLayers.push({ ...layer, dataURL })
        }

        return compressedLayers
    }

    private async decompressLayers<T extends { dataURL: Buffer }>(
        layers: CompressedLayer<T>[]
    ): Promise<DecompressedLayer<T>[] | undefined> {
        const decompressedLayers: DecompressedLayer<T>[] = []

        for (const layer of layers) {
            const dataURL = await this.decompressDataURL(layer.dataURL)
            if (!dataURL) return
            decompressedLayers.push({ ...layer, dataURL })
        }

        return decompressedLayers
    }

    private async compressDataURL(dataURL: string): Promise<Buffer | undefined> {
        try {
            console.log('compressDataURL', { dataURL })
            /*const compStream = new CompressionStream(encoding)
            const writer = compStream.writable.getWriter()
            const encoded = new TextEncoder().encode(string)
            await writer.write(encoded)
            await writer.close()
            const result = await new Response(compStream.readable).arrayBuffer()*/
            const base64 = this.dataURLToBase64(dataURL)
            const buffer = Buffer.from(base64, 'base64')
            console.log('compressDataURL result', { buffer })
            return buffer
        } catch (e) {
            console.error('compressDataURL', e)
        }
    }

    private async decompressDataURL(buffer: Buffer): Promise<string | undefined> {
        try {
            console.log('decompressDataURL', { buffer })
            /*const decompStream = new DecompressionStream(encoding)
            const writer = decompStream.writable.getWriter()
            await writer.write(buffer)
            await writer.close()
            const result = await new Response(decompStream.readable).arrayBuffer().then((arrayBuffer) => {
                return new TextDecoder().decode(arrayBuffer)
            })*/
            const base64 = buffer.toString('base64')
            const dataURL = this.base64ToDataURL(base64)
            console.log('decompressDataURL', { dataURL })
            return dataURL
        } catch (e) {
            console.error('decompressDataURL', e)
        }
    }

    private dataURLToBase64(str: string): string {
        return str.replace('data:image/png;base64,', '')
    }

    private base64ToDataURL(str: string): string {
        return 'data:image/png;base64,' + str
    }
}
