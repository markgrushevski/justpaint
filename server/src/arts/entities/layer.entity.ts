import { Column, Entity, JoinColumn, ManyToOne, PrimaryGeneratedColumn } from 'typeorm'
import { ArtEntity } from './art.entity'

@Entity('layers')
export class LayerEntity {
    @PrimaryGeneratedColumn('uuid')
    id: string | undefined

    @Column({ type: 'uuid' })
    artId: string

    @Column({ type: 'varchar', length: 64 })
    name: string

    @Column({ type: 'bytea' })
    dataURL: ArrayBuffer

    @ManyToOne(() => ArtEntity, (art) => art.id)
    @JoinColumn({ name: 'art_id', referencedColumnName: 'id' })
    art?: ArtEntity
}
