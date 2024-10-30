import { Injectable } from '@nestjs/common'
import { InjectRepository } from '@nestjs/typeorm'
import { Repository } from 'typeorm'
import { SaveArtDto } from './dto/arts.dto'
import { ArtEntity } from './entities/art.entity'
import { ArtsRelations, CompressedLayer, DecompressedLayer } from './types/arts.types'
import { LayersService } from './services/layers.service'
import { LayerEntity } from './entities/layer.entity'

@Injectable()
export class ArtsService {
    constructor(
        @InjectRepository(ArtEntity)
        private readonly artsRepository: Repository<ArtEntity>,
        private readonly layersService: LayersService,
    ) {}

    async get(userId: string): Promise<ArtEntity[]> {
        const relations: ArtsRelations = ['layers']
        const arts = await this.artsRepository.find({ where: { userId }, relations })

        return Promise.all(
            arts.map(async (art) => {
                art.layers = await this.decompressLayers(art.layers ?? [])
                return art
            }),
        )
    }

    async save(userId: string, art: SaveArtDto): Promise<ArtEntity> {
        const savedArt = await this.artsRepository.save({
            ...art,
            userId,
        })

        const compressedLayers = await this.compressLayers(art.layers)
        const layers = compressedLayers.map((layer) => {
            layer.artId = savedArt.id!
            return layer as LayerEntity
        })
        const savedLayers = await this.layersService.save(layers)

        return { ...savedArt, layers: savedLayers }
    }

    async delete(userId: string, artId: string): Promise<boolean> {
        await this.artsRepository.delete({ id: artId, userId })
        return true
    }

    private async compressLayers<T extends object>(layers: DecompressedLayer<T>[]): Promise<CompressedLayer<T>[]> {
        return Promise.all(
            layers.map(async (layer) => {
                return {
                    ...layer,
                    dataURL: await this.compressString(layer.dataURL),
                }
            }),
        )
    }

    private async decompressLayers<T extends object>(layers: CompressedLayer<T>[]): Promise<DecompressedLayer<T>[]> {
        return Promise.all(
            layers.map(async (layer) => {
                return {
                    ...layer,
                    dataURL: await this.decompressString(layer.dataURL),
                }
            }),
        )
    }

    private async compressString(string: string, encoding: CompressionFormat = 'gzip'): Promise<ArrayBuffer> {
        console.log('compressString', { string })
        const compStream = new CompressionStream(encoding)
        const writer = compStream.writable.getWriter()
        const encoded = new TextEncoder().encode(string)
        console.log('compressString', { encoded })
        writer.write(encoded)
        writer.close()
        return new Response(compStream.readable).arrayBuffer()
    }

    private async decompressString(byteArray: ArrayBuffer, encoding: CompressionFormat = 'gzip'): Promise<string> {
        console.log('decompressString', { byteArray })
        const decompStream = new DecompressionStream(encoding)
        const writer = decompStream.writable.getWriter()
        writer.write(byteArray)
        writer.close()
        return new Response(decompStream.readable).arrayBuffer().then((arrayBuffer) => {
            const decoded = new TextDecoder().decode(arrayBuffer)
            console.log('decompressString', { decoded })
            return decoded
        })
    }
}
