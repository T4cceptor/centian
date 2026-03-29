export const unauthorizedAuthHeaderHint = "X-Centian-Auth-Header";
export const defaultApiAuthHeaderName = "X-Centian-Auth";

const storageKey = "centian.ui.api-auth";
let memoryStoredApiAuth: StoredApiAuth | undefined;

export type StoredApiAuth = {
  headerName: string;
  apiKey: string;
};

export function loadStoredApiAuth(): StoredApiAuth | undefined {
  const storage = getLocalStorage();
  if (!storage) {
    return memoryStoredApiAuth;
  }

  const raw = storage.getItem(storageKey);
  if (!raw) {
    return undefined;
  }

  try {
    const parsed = JSON.parse(raw) as Partial<StoredApiAuth>;
    const headerName = parsed.headerName?.trim();
    const apiKey = parsed.apiKey?.trim();
    if (!headerName || !apiKey) {
      return undefined;
    }
    return { headerName, apiKey };
  } catch {
    return memoryStoredApiAuth;
  }
}

export function saveStoredApiAuth(auth: StoredApiAuth): void {
  memoryStoredApiAuth = {
    headerName: auth.headerName.trim(),
    apiKey: auth.apiKey.trim(),
  };
  const storage = getLocalStorage();
  if (!storage) {
    return;
  }

  const { headerName, apiKey } = memoryStoredApiAuth;
  if (!headerName || !apiKey) {
    clearStoredApiAuth();
    return;
  }

  storage.setItem(storageKey, JSON.stringify({ headerName, apiKey }));
}

export function clearStoredApiAuth(): void {
  memoryStoredApiAuth = undefined;
  const storage = getLocalStorage();
  if (!storage) {
    return;
  }
  storage.removeItem(storageKey);
}

function getLocalStorage(): Pick<Storage, "getItem" | "setItem" | "removeItem"> | undefined {
  if (typeof window === "undefined" || !window.localStorage) {
    return undefined;
  }

  const storage = window.localStorage as Partial<Storage>;
  if (
    typeof storage.getItem !== "function" ||
    typeof storage.setItem !== "function" ||
    typeof storage.removeItem !== "function"
  ) {
    return undefined;
  }
  return storage as Pick<Storage, "getItem" | "setItem" | "removeItem">;
}
