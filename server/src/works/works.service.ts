import { Injectable } from '@nestjs/common';

@Injectable()
export class WorksService {
  async getWorksByAccount() {
    return [
      { id: '1', name: 'New work 1', createdAt: '00.00.00' },
      { id: '2', name: 'New work 2', createdAt: '00.00.00' },
      { id: '3', name: 'New work 3', createdAt: '00.00.00' },
      { id: '4', name: 'New work 4', createdAt: '00.00.00' },
      { id: '5', name: 'New work 5', createdAt: '00.00.00' },
      { id: '6', name: 'New work 6', createdAt: '00.00.00' },
      { id: '7', name: 'New work 7', createdAt: '00.00.00' },
      { id: '8', name: 'New work 8', createdAt: '00.00.00' },
      { id: '9', name: 'New work 9', createdAt: '00.00.00' },
      { id: '10', name: 'New work 10', createdAt: '00.00.00' },
      { id: '11', name: 'New work 11', createdAt: '00.00.00' },
      { id: '12', name: 'New work 12', createdAt: '00.00.00' },
    ];
  }
}
