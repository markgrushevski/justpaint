import { Controller, Get } from '@nestjs/common';
import { WorksService } from './works.service';

@Controller('works')
export class WorksController {
  constructor(private readonly worksService: WorksService) {}

  @Get()
  async getWorksByAccount() {
    return this.worksService.getWorksByAccount();
  }
}
