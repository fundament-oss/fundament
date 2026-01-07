const CONFIG = {
  apiBaseUrl: 'http://authn.127.0.0.1.nip.io:8080',
  servicePath: '/authn.v1.AuthnService',
};

export async function connectRpc<T>(method: string, request: object = {}): Promise<T> {
  const url = `${CONFIG.apiBaseUrl}${CONFIG.servicePath}/${method}`;

  const response = await fetch(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    credentials: 'include', // Important: send cookies with requests
    body: JSON.stringify(request),
  });

  if (!response.ok) {
    const error = await response.json().catch(() => ({ message: response.statusText }));
    throw new Error(error.message || `Request failed: ${response.status}`);
  }

  return response.json();
}

export interface UserInfo {
  id: string;
  tenantId: string;
  name: string;
  externalId: string;
  groups: string[];
}

export interface UserResponse {
  user: UserInfo;
}

export function getLoginUrl(): string {
  return `${CONFIG.apiBaseUrl}/login`;
}

export async function getUserInfo(): Promise<UserInfo> {
  const response = await connectRpc<UserResponse>('GetUserInfo', {});
  return response.user;
}

export async function refreshToken(): Promise<void> {
  const response = await fetch(`${CONFIG.apiBaseUrl}/refresh`, {
    method: 'POST',
    credentials: 'include',
  });

  if (!response.ok) {
    const error = await response.json().catch(() => ({ message: response.statusText }));
    throw new Error(error.message || `Request failed: ${response.status}`);
  }
}

export async function logout(): Promise<void> {
  const response = await fetch(`${CONFIG.apiBaseUrl}/logout`, {
    method: 'POST',
    credentials: 'include',
  });

  if (!response.ok) {
    throw new Error(`Logout failed: ${response.status}`);
  }
}
