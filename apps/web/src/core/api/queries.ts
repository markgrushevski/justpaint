import { useQuery } from '@tanstack/vue-query'
import { mainAPI } from './api.ts'
import type { Art } from './types.ts'

export function useArtsQuery() {
    return useQuery<Art[]>({
        queryKey: ['arts'],
        queryFn: mainAPI.arts.getArts,
        enabled: false
    })
}
