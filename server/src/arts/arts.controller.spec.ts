import { Test, TestingModule } from '@nestjs/testing'
import { ArtsController } from './arts.controller'

describe('ArtsController', () => {
    let controller: ArtsController

    beforeEach(async () => {
        const module: TestingModule = await Test.createTestingModule({
            controllers: [ArtsController]
        }).compile()

        controller = module.get<ArtsController>(ArtsController)
    })

    it('should be defined', () => {
        expect(controller).toBeDefined()
    })
})
