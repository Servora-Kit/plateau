import { createUserServiceClient, type User } from '@/api/generated/example/service/v1'
import { transport } from '@/api/transport'

export const userApi = createUserServiceClient(transport)
export type { User }
