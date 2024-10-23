import { Injectable } from '@nestjs/common';

@Injectable()
export class WorksService {
  async getWorksByAccount() {
    return [{ id: '0' }];
  }
}
