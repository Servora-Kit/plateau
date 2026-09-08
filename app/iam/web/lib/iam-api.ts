import { createAccountServiceClient } from "@plateau/api/iam/account/v1"
import { createAuthnServiceClient } from "@plateau/api/iam/authn/v1"
import { createSessionServiceClient } from "@plateau/api/iam/session/v1"
import { createHttpClient, createTransport } from "@plateau/client"

export function createIamApi(signal?: AbortSignal) {
  const transport = createTransport(createHttpClient({ service: "iam" }), {
    signal,
  })

  return {
    account: createAccountServiceClient(transport),
    authn: createAuthnServiceClient(transport),
    session: createSessionServiceClient(transport),
  }
}
