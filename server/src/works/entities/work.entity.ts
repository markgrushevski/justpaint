import { Column, Entity, PrimaryGeneratedColumn } from 'typeorm';

@Entity('works')
export class WorkEntity {
  @PrimaryGeneratedColumn({ type: 'int' })
  id?: number;

  @Column({ type: 'varchar', length: 120, unique: true })
  workId: number;

  @Column({ type: 'varchar', length: 120 })
  userId: number;

  @Column({ type: 'text' })
  canvasDataURL: string;

  @Column({ type: 'timestamp', default: () => 'CURRENT_TIMESTAMP' })
  createdAt: Date;

  @Column({ type: 'timestamp', default: () => 'CURRENT_TIMESTAMP' })
  updatedAt: Date;
}
