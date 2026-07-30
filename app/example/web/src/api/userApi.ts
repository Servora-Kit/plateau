import { createUserHTTPServiceClient, type User } from '@/api/generated/example/service/v1'
import { transport } from '@/api/transport'

export const userApi = createUserHTTPServiceClient(transport)
export type { User }
