import { Module } from '@nestjs/common'
import { TypeOrmModule } from '@nestjs/typeorm'
import { ArtEntity } from './entities/art.entity'
import { ArtsService } from './arts.service'
import { ArtsController } from './arts.controller'
import { LayerEntity } from './entities/layer.entity'
import { LayersService } from './services/layers.service'

@Module({
    imports: [TypeOrmModule.forFeature([ArtEntity, LayerEntity])],
    controllers: [ArtsController],
    providers: [ArtsService, LayersService],
    exports: [ArtsService, LayersService],
})
export class ArtsModule {}
