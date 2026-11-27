import { createAccountServiceClient } from "@plateau/api/plateau/iam/account/v1"
import { createAuthnServiceClient } from "@plateau/api/plateau/iam/authn/v1"
import { createSessionServiceClient } from "@plateau/api/plateau/iam/session/v1"
import { createHttpClient, createTransport } from "@servora/plateau-utils/client"

export function createIamApi(signal?: AbortSignal) {
  const transport = createTransport(
    createHttpClient({ service: "iam", cache: "no-store" }),
    {
      signal,
    }
  )

  return {
    account: createAccountServiceClient(transport),
    authn: createAuthnServiceClient(transport),
    session: createSessionServiceClient(transport),
  }
}
