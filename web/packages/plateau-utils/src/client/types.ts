export type {
  ClientTransport,
  DuplexStream,
  ServerStream,
  TransportMeta,
} from "@servora/proto-utils/transport";

export interface StreamClient {
  readonly baseURL: string;
  readonly headers: Headers;
  readonly credentials: RequestCredentials;
  readonly timeout: number;
  readonly fetch: typeof globalThis.fetch;
}
