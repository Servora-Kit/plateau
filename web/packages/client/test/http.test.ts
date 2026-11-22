import assert from "node:assert/strict";
import { createServer, type RequestListener } from "node:http";
import { test } from "node:test";
import { setTimeout as sleep } from "node:timers/promises";
import { ApiError } from "@servora/proto-utils/errors";
import { createHttpClient, createTransport } from "../src/index.ts";

async function serve(handler: RequestListener) {
  const server = createServer(handler);
  const { promise, resolve, reject } = Promise.withResolvers<string>();
  server.once("error", reject);
  server.listen(0, "127.0.0.1", () => {
    const address = server.address();
    if (!address || typeof address === "string")
      return reject(new Error("监听地址不可用"));
    resolve(`http://127.0.0.1:${address.port}`);
  });
  return {
    baseURL: await promise,
    close() {
      server.closeAllConnections();
      server.close();
    },
  };
}

test("暂时失败只自动重试读取，不重提交写入或冲突", async () => {
  const counts = new Map<string, number>();
  const server = await serve((request, response) => {
    const key = `${request.method} ${request.url}`;
    const count = (counts.get(key) ?? 0) + 1;
    counts.set(key, count);
    response.setHeader("Content-Type", "application/json");
    response.statusCode =
      request.url === "/conflict" ? 409 : count === 1 ? 503 : 200;
    response.end(
      JSON.stringify(
        response.statusCode === 200
          ? { restored: true }
          : {
              code: response.statusCode,
              reason: "TEMPORARY",
              message: "暂时不可用",
            },
      ),
    );
  });
  try {
    const client = createHttpClient({
      service: "检查",
      baseURL: server.baseURL,
    });
    assert.deepEqual(await client.request("/read"), { restored: true });
    await assert.rejects(
      client.request("/write", {
        method: "POST",
        body: { value: "仅提交一次" },
      }),
      (error: unknown) =>
        error instanceof ApiError &&
        error.kind === "http" &&
        error.httpStatus === 503,
    );
    await assert.rejects(
      client.request("/conflict"),
      (error: unknown) => error instanceof ApiError && error.httpStatus === 409,
    );
    assert.equal(counts.get("GET /read"), 2);
    assert.equal(counts.get("POST /write"), 1);
    assert.equal(counts.get("GET /conflict"), 1);
  } finally {
    server.close();
  }
});

test("调用方 signal 不会关闭请求超时，响应体也受超时约束", async () => {
  const server = await serve((_request, response) => {
    response.writeHead(200, { "Content-Type": "application/json" });
    response.flushHeaders();
    response.write('{"waiting":');
  });
  try {
    const controller = new AbortController();
    const client = createHttpClient({
      service: "检查",
      baseURL: server.baseURL,
      timeout: 80,
    });
    await assert.rejects(
      client.request("/slow", { signal: controller.signal }),
      (error: unknown) => error instanceof ApiError && error.kind === "timeout",
    );
    assert.equal(controller.signal.aborted, false);
  } finally {
    server.close();
  }
});

test("重试等待期间取消不会再次请求且保留取消分类", async () => {
  let requests = 0;
  const { promise: firstRequest, resolve: received } =
    Promise.withResolvers<void>();
  const server = await serve((_request, response) => {
    requests++;
    response.writeHead(503, { "Content-Type": "application/json" });
    response.end('{"code":503,"reason":"TEMPORARY","message":"稍后重试"}');
    received();
  });
  try {
    const controller = new AbortController();
    const client = createHttpClient({
      service: "检查",
      baseURL: server.baseURL,
      retryDelay: 100,
    });
    const pending = client.request("/retry", { signal: controller.signal });
    const rejected = assert.rejects(
      pending,
      (error: unknown) =>
        error instanceof ApiError && error.kind === "cancelled",
    );
    await firstRequest;
    controller.abort("页面已离开");
    await rejected;
    await sleep(130);
    assert.equal(requests, 1);
  } finally {
    server.close();
  }
});

test("嵌套页面的无前导斜杠 API 路径仍从站点根目录解析", async (context) => {
  const server = await serve((request, response) => {
    response.setHeader("Content-Type", "application/json");
    if (
      request.url !== "/v1/register" &&
      request.url !== "/gateway/v1/register"
    ) {
      response.writeHead(404);
      response.end('{"reason":"WRONG_PATH"}');
      return;
    }
    response.end('{"registered":true}');
  });
  const previousWindow = Object.getOwnPropertyDescriptor(globalThis, "window");
  const pageURL = new URL("/register/", server.baseURL);
  Object.defineProperty(globalThis, "window", {
    configurable: true,
    value: { location: pageURL },
  });
  context.after(() => {
    if (previousWindow)
      Object.defineProperty(globalThis, "window", previousWindow);
    else Reflect.deleteProperty(globalThis, "window");
    server.close();
  });
  const client = createHttpClient({
    service: "浏览器路径",
    fetch: (input, init) =>
      fetch(typeof input === "string" ? new URL(input, pageURL) : input, init),
  });
  assert.deepEqual(await client.request("v1/register", { method: "POST" }), {
    registered: true,
  });
  assert.deepEqual(
    await client.request("v1/register", {
      method: "POST",
      baseURL: "/gateway",
    }),
    { registered: true },
  );
});

test("createTransport 为无请求体非安全方法附带 JSON Content-Type 且发送空报文，GET 不附带", async () => {
  const captured: Array<{
    method?: string;
    url?: string;
    contentType?: string;
    accept?: string;
    rawBody: string;
  }> = [];
  const server = await serve((request, response) => {
    const chunks: Buffer[] = [];
    request.on("data", (chunk: Buffer) => chunks.push(chunk));
    request.on("end", () => {
      captured.push({
        method: request.method,
        url: request.url,
        contentType: request.headers["content-type"],
        accept: request.headers.accept,
        rawBody: Buffer.concat(chunks).toString("utf8"),
      });
      response.writeHead(200, { "Content-Type": "application/json" });
      response.end("{}");
    });
  });
  try {
    const client = createHttpClient({
      service: "admin",
      baseURL: server.baseURL,
      retry: 0,
    });
    const transport = createTransport(client);
    await transport.unary("/v1/admin/auth/session", "GET", null, {
      service: "AuthService",
      method: "GetSession",
    });
    await transport.unary("/v1/admin/users/u_001", "DELETE", null, {
      service: "UserService",
      method: "DeleteUser",
    });
    await transport.unary("/v1/admin/users/u_001:revokeSessions", "POST", null, {
      service: "UserService",
      method: "RevokeUserSessions",
    });

    const clientWithDefaultCT = createHttpClient({
      service: "admin",
      baseURL: server.baseURL,
      headers: { "Content-Type": "application/json" },
    });
    await clientWithDefaultCT.request("/v1/admin/check", {
      method: "GET",
      body: null,
    });

    assert.deepEqual(captured, [
      {
        method: "GET",
        url: "/v1/admin/auth/session",
        contentType: undefined,
        accept: "application/json",
        rawBody: "",
      },
      {
        method: "DELETE",
        url: "/v1/admin/users/u_001",
        contentType: "application/json",
        accept: "application/json",
        rawBody: "",
      },
      {
        method: "POST",
        url: "/v1/admin/users/u_001:revokeSessions",
        contentType: "application/json",
        accept: "application/json",
        rawBody: "",
      },
      {
        method: "GET",
        url: "/v1/admin/check",
        contentType: undefined,
        accept: "*/*",
        rawBody: "",
      },
    ]);
  } finally {
    server.close();
  }
});
