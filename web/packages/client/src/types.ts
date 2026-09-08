export interface TransportMeta {
  service: string;
  method: string;
}

export interface ServerStream<T> {
  onEvent(listener: (data: T) => void): () => void;
  onError(handler: (error: Error) => void): void;
  close(): void;
}

export interface DuplexStream<TIn, TOut> extends ServerStream<TOut> {
  send(data: TIn): void;
  closeSend(): void;
}

export interface StreamClient {
  readonly baseURL: string;
  readonly headers: Headers;
  readonly credentials: RequestCredentials;
  readonly timeout: number;
  readonly fetch: typeof globalThis.fetch;
}
