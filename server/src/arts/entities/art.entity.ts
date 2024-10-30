import {
    Column,
    CreateDateColumn,
    Entity,
    JoinColumn,
    ManyToOne,
    OneToMany,
    PrimaryGeneratedColumn,
    UpdateDateColumn,
} from 'typeorm'
import { Exclude } from 'class-transformer'
import { UserEntity } from '../../users/entities/user.entity'
import { LayerEntity } from './layer.entity'

@Entity('arts')
export class ArtEntity {
    @PrimaryGeneratedColumn('uuid')
    id: string | undefined

    @Exclude()
    @Column({ type: 'uuid' })
    userId: string

    @Column({ type: 'varchar', length: 64 })
    name: string

    @CreateDateColumn()
    createdAt: Date

    @UpdateDateColumn()
    updatedAt: Date

    @OneToMany(() => LayerEntity, (layer) => layer.artId)
    @JoinColumn({ name: 'id', referencedColumnName: 'art_id' })
    layers?: LayerEntity[]

    @ManyToOne(() => UserEntity, (user) => user.id)
    @JoinColumn({ name: 'user_id', referencedColumnName: 'id' })
    user?: UserEntity
}
