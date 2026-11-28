import type {
  CreateOAuthClientRequest,
  CreateOAuthClientResponse,
  CreateOAuthClientSecretRequest,
  CreateOAuthClientSecretResponse,
  DeleteOAuthClientRequest,
  DeleteOAuthClientSecretRequest,
  DisableOAuthClientRequest,
  EnableOAuthClientRequest,
  GetOAuthClientRequest,
  ListOAuthClientsRequest,
  ListOAuthClientsResponse,
  OAuthClientService,
  UpdateOAuthClientRequest,
} from '@plateau/api/plateau/admin/client/v1';
import type { OAuthClient } from '@plateau/api/plateau/iam/client/v1';

import { createAdminApi } from './client';

export function createOAuthClientsApi(
  signal?: AbortSignal,
): OAuthClientService {
  return createAdminApi(signal).oauthClients;
}

export function listOAuthClients(
  request: ListOAuthClientsRequest,
  signal?: AbortSignal,
): Promise<ListOAuthClientsResponse> {
  return createOAuthClientsApi(signal).ListOAuthClients(request);
}

export function getOAuthClient(
  request: GetOAuthClientRequest,
  signal?: AbortSignal,
): Promise<OAuthClient> {
  return createOAuthClientsApi(signal).GetOAuthClient(request);
}

export function createOAuthClient(
  request: CreateOAuthClientRequest,
  signal?: AbortSignal,
): Promise<CreateOAuthClientResponse> {
  return createOAuthClientsApi(signal).CreateOAuthClient(request);
}

export function updateOAuthClient(
  request: UpdateOAuthClientRequest,
  signal?: AbortSignal,
): Promise<OAuthClient> {
  return createOAuthClientsApi(signal).UpdateOAuthClient(request);
}

export function disableOAuthClient(
  request: DisableOAuthClientRequest,
  signal?: AbortSignal,
): Promise<OAuthClient> {
  return createOAuthClientsApi(signal).DisableOAuthClient(request);
}

export function enableOAuthClient(
  request: EnableOAuthClientRequest,
  signal?: AbortSignal,
): Promise<OAuthClient> {
  return createOAuthClientsApi(signal).EnableOAuthClient(request);
}

export function deleteOAuthClient(
  request: DeleteOAuthClientRequest,
  signal?: AbortSignal,
): Promise<Record<never, never>> {
  return createOAuthClientsApi(signal).DeleteOAuthClient(request);
}

export function createOAuthClientSecret(
  request: CreateOAuthClientSecretRequest,
  signal?: AbortSignal,
): Promise<CreateOAuthClientSecretResponse> {
  return createOAuthClientsApi(signal).CreateOAuthClientSecret(request);
}

export function deleteOAuthClientSecret(
  request: DeleteOAuthClientSecretRequest,
  signal?: AbortSignal,
): Promise<Record<never, never>> {
  return createOAuthClientsApi(signal).DeleteOAuthClientSecret(request);
}
