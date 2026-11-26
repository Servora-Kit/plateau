import assert from "node:assert/strict";
import { test } from "node:test";
import {
  CrudErrorReason,
  isCrudErrorReason,
} from "@servora/plateau-utils/error-reasons/plateau/crud/v1/errors";
import {
  isSecurityErrorReason,
  SecurityErrorReason,
} from "@servora/plateau-utils/error-reasons/plateau/security/errors/v1/errors";

const expectedCrudReasons = {
  CRUD_ERROR_REASON_UNSPECIFIED: "CRUD_ERROR_REASON_UNSPECIFIED",
  CRUD_ERROR_REASON_INVALID_RESOURCE_NAME:
    "CRUD_ERROR_REASON_INVALID_RESOURCE_NAME",
  CRUD_ERROR_REASON_INVALID_PAGE_TOKEN: "CRUD_ERROR_REASON_INVALID_PAGE_TOKEN",
  CRUD_ERROR_REASON_INVALID_FILTER: "CRUD_ERROR_REASON_INVALID_FILTER",
  CRUD_ERROR_REASON_INVALID_ORDER_BY: "CRUD_ERROR_REASON_INVALID_ORDER_BY",
  CRUD_ERROR_REASON_INVALID_FIELD_MASK: "CRUD_ERROR_REASON_INVALID_FIELD_MASK",
  CRUD_ERROR_REASON_INVALID_FIELD_VALUE:
    "CRUD_ERROR_REASON_INVALID_FIELD_VALUE",
  CRUD_ERROR_REASON_INTERNAL: "CRUD_ERROR_REASON_INTERNAL",
} as const;

const expectedSecurityReasons = {
  SECURITY_ERROR_REASON_UNSPECIFIED: "SECURITY_ERROR_REASON_UNSPECIFIED",
  SECURITY_ERROR_REASON_UNAUTHENTICATED: "SECURITY_ERROR_REASON_UNAUTHENTICATED",
  SECURITY_ERROR_REASON_PERMISSION_DENIED:
    "SECURITY_ERROR_REASON_PERMISSION_DENIED",
  SECURITY_ERROR_REASON_INVALID_ARGUMENT:
    "SECURITY_ERROR_REASON_INVALID_ARGUMENT",
  SECURITY_ERROR_REASON_UNAVAILABLE: "SECURITY_ERROR_REASON_UNAVAILABLE",
  SECURITY_ERROR_REASON_INTERNAL: "SECURITY_ERROR_REASON_INTERNAL",
} as const;

function narrowCrudReason(value: unknown): CrudErrorReason | undefined {
  if (isCrudErrorReason(value)) {
    const narrowed: CrudErrorReason = value;
    return narrowed;
  }
  return undefined;
}

function narrowSecurityReason(value: unknown): SecurityErrorReason | undefined {
  if (isSecurityErrorReason(value)) {
    const narrowed: SecurityErrorReason = value;
    return narrowed;
  }
  return undefined;
}

test("error-reasons 子路径导出完整的 CRUD 与 Security 运行时枚举和收窄守卫", () => {
  assert.deepEqual(CrudErrorReason, expectedCrudReasons);
  assert.deepEqual(SecurityErrorReason, expectedSecurityReasons);

  for (const reason of Object.values(expectedCrudReasons)) {
    assert.equal(isCrudErrorReason(reason), true);
    assert.equal(narrowCrudReason(reason), reason);
  }
  for (const reason of Object.values(expectedSecurityReasons)) {
    assert.equal(isSecurityErrorReason(reason), true);
    assert.equal(narrowSecurityReason(reason), reason);
  }
});

test("错误原因守卫拒绝非枚举值与原型链属性", () => {
  const invalidReasons: unknown[] = [
    "CRUD_ERROR_REASON_UNKNOWN",
    "crud_error_reason_internal",
    "SECURITY_ERROR_REASON_UNKNOWN",
    "toString",
    null,
    500,
    {},
  ];

  for (const reason of invalidReasons) {
    assert.equal(isCrudErrorReason(reason), false);
    assert.equal(narrowCrudReason(reason), undefined);
    assert.equal(isSecurityErrorReason(reason), false);
    assert.equal(narrowSecurityReason(reason), undefined);
  }
});

test("内部生成目录与已从 @servora/proto-utils 移除的旧子路径均拒绝导出", async () => {
  const hiddenPaths = [
    "@servora/plateau-utils/dist/gen/plateau/crud/v1/errors.errors.js",
    "@servora/plateau-utils/src/gen/plateau/crud/v1/errors.errors.ts",
    "@servora/proto-utils/crud",
  ];

  for (const specifier of hiddenPaths) {
    await assert.rejects(
      import(specifier),
      (error: unknown) =>
        typeof error === "object" &&
        error !== null &&
        "code" in error &&
        error.code === "ERR_PACKAGE_PATH_NOT_EXPORTED",
    );
  }
});

test("全部公开子路径均可正常动态解析", async () => {
  const [root, client, crud, crudErrors, securityErrors] = await Promise.all([
    import("@servora/plateau-utils"),
    import("@servora/plateau-utils/client"),
    import("@servora/plateau-utils/crud"),
    import("@servora/plateau-utils/error-reasons/plateau/crud/v1/errors"),
    import(
      "@servora/plateau-utils/error-reasons/plateau/security/errors/v1/errors"
    ),
  ]);

  assert.equal(typeof root.createHttpClient, "function");
  assert.equal(typeof client.createTransport, "function");
  assert.equal(typeof crud.makeUpdateMask, "function");
  assert.equal(typeof crudErrors.isCrudErrorReason, "function");
  assert.equal(typeof securityErrors.isSecurityErrorReason, "function");
});
