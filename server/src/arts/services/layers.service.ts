import { Injectable } from '@nestjs/common'
import { InjectRepository } from '@nestjs/typeorm'
import { Repository } from 'typeorm'
import { LayerEntity } from '../entities/layer.entity'

@Injectable()
export class LayersService {
    constructor(
        @InjectRepository(LayerEntity)
        private readonly layersRepository: Repository<LayerEntity>,
    ) {}

    async get(artId: string): Promise<LayerEntity[]> {
        return this.layersRepository.find({ where: { artId } })
    }

    async save(layers: LayerEntity[]): Promise<LayerEntity[]> {
        return this.layersRepository.save(layers)
    }
}
