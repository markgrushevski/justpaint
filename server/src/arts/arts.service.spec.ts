import { Test, TestingModule } from '@nestjs/testing'
import { ArtsService } from './arts.service'

describe('ArtsService', () => {
    let service: ArtsService

    beforeEach(async () => {
        const module: TestingModule = await Test.createTestingModule({
            providers: [ArtsService],
        }).compile()

        service = module.get<ArtsService>(ArtsService)
    })

    it('should be defined', () => {
        expect(service).toBeDefined()
    })
})
