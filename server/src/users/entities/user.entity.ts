import { Column, Entity, JoinColumn, OneToMany, PrimaryGeneratedColumn } from 'typeorm'
import { ArtEntity } from '../../arts/entities/art.entity'
import { Exclude } from 'class-transformer'

@Entity('users')
export class UserEntity {
    @PrimaryGeneratedColumn('uuid')
    id: string

    @Column({ type: 'varchar', length: 20, unique: true })
    nickname: string

    @Column({ type: 'varchar', length: 32 })
    password: string

    @OneToMany(() => ArtEntity, (art) => art.userId)
    @JoinColumn({ name: 'id', referencedColumnName: 'user_id' })
    arts?: ArtEntity[]
}
