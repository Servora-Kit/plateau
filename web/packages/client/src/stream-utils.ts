import { ApiError } from "@servora/proto-utils/errors";
import type { TransportMeta } from "./types.ts";

export class StreamCallbacks<T> {
  private readonly listeners = new Set<(data: T) => void>();
  private errorHandler?: (error: Error) => void;
  private terminalError?: Error;
  private errorScheduled = false;
  private errorDelivered = false;
  private ended = false;

  onEvent(listener: (data: T) => void): () => void {
    if (this.ended) return () => {};
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  onError(handler: (error: Error) => void): void {
    if (this.errorDelivered || (this.ended && !this.terminalError)) return;
    this.errorHandler = handler;
    this.scheduleError();
  }

  emit(data: T): void {
    if (this.ended) return;
    for (const listener of this.listeners) {
      if (this.ended) break;
      try {
        listener(data);
      } catch (error) {
        queueMicrotask(() => {
          throw error;
        });
      }
    }
  }

  fail(error: Error): void {
    this.ended = true;
    if (this.terminalError) return;
    this.terminalError = error;
    this.listeners.clear();
    this.scheduleError();
  }

  finish(): void {
    this.ended = true;
    this.listeners.clear();
    this.errorHandler = undefined;
  }

  private scheduleError(): void {
    if (
      !this.terminalError ||
      !this.errorHandler ||
      this.errorScheduled ||
      this.errorDelivered
    )
      return;
    this.errorScheduled = true;
    queueMicrotask(() => {
      this.errorScheduled = false;
      if (!this.terminalError || !this.errorHandler || this.errorDelivered)
        return;
      this.errorDelivered = true;
      const handler = this.errorHandler;
      this.errorHandler = undefined;
      try {
        handler(this.terminalError);
      } catch (error) {
        queueMicrotask(() => {
          throw error;
        });
      }
    });
  }
}

export function streamApiError(
  kind: "http" | "network" | "timeout" | "cancelled",
  message: string,
  meta: TransportMeta,
  options: {
    httpStatus?: number;
    responseBody?: unknown;
    cause?: unknown;
  } = {},
): ApiError {
  return new ApiError({ kind, message, ...meta, ...options });
}

export function asError(value: unknown, message: string): Error {
  return new Error(message, { cause: value });
}

export function resolveBrowserURL(baseURL: string, path: string): URL {
  if (typeof location === "undefined")
    throw new Error("流式客户端只能在浏览器中使用");
  if (/^[a-z][a-z\d+.-]*:/iu.test(path)) return new URL(path);

  const base = new URL(baseURL, `${location.origin}/`);
  if (path.startsWith("?") || path.startsWith("#")) return new URL(path, base);

  const suffixIndex = path.search(/[?#]/u);
  const pathPart = suffixIndex === -1 ? path : path.slice(0, suffixIndex);
  const suffix = suffixIndex === -1 ? "" : path.slice(suffixIndex);
  const basePath = base.pathname.endsWith("/")
    ? base.pathname
    : `${base.pathname}/`;
  const relativePath = pathPart.replace(/^\/+/, "");
  const pathname = `${basePath}${relativePath}`.replace(/\/{2,}/gu, "/");
  return new URL(`${pathname}${suffix}`, base.origin);
}
