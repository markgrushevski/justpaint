import { BadRequestException, Injectable, NotFoundException, UnauthorizedException } from '@nestjs/common'
import { InjectRepository } from '@nestjs/typeorm'
import { Repository } from 'typeorm'
import { UserEntity } from './entities/user.entity'
import { ArtsService } from '../arts/arts.service'

@Injectable()
export class UsersService {
    constructor(
        @InjectRepository(UserEntity)
        private readonly userRepository: Repository<UserEntity>,
    ) {}

    async checkNickname(nickname: string): Promise<string> {
        const existedUser = await this.userRepository.findOne({ where: { nickname } })
        if (existedUser) return nickname
        else throw new NotFoundException()
    }

    async getByNickname(nickname: string) {
        return this.userRepository.findOne({ where: { nickname } })
    }

    /**
     * @return user id
     * */
    async create(nickname: string, password: string): Promise<string> {
        const res = await this.userRepository.insert({
            nickname,
            password,
        })

        return res.identifiers[0].id
    }
}
