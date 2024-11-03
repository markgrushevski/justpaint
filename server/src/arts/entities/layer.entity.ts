import { Buffer } from 'node:buffer'
import { Column, Entity, JoinColumn, ManyToOne, PrimaryGeneratedColumn } from 'typeorm'
import { ArtEntity } from './art.entity'

@Entity('layers')
export class LayerEntity {
    @PrimaryGeneratedColumn('uuid')
    id: string

    @Column({ type: 'uuid' })
    artId: string

    @Column({ type: 'varchar', length: 64 })
    name: string

    @Column({ type: 'bytea' })
    dataURL: Buffer

    @ManyToOne(() => ArtEntity, (art) => art.id)
    @JoinColumn({ name: 'art_id', referencedColumnName: 'id' })
    art?: ArtEntity
}
