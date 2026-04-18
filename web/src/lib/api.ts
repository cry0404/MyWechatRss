import { useAuthStore } from "@/stores/authStore";

const API_BASE = import.meta.env.VITE_API_BASE || "";

export interface ApiError {
  message: string;
  status?: number;
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  options?: { noAuth?: boolean }
): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };

  if (!options?.noAuth) {
    const token = localStorage.getItem("token");
    if (token) headers["Authorization"] = `Bearer ${token}`;
  }

  // 不带浏览器 Cookie：会话只认 Authorization Bearer（localStorage）。
  // 否则旧的 session cookie 会与当前 token 冲突，后端曾优先读 cookie 会导致鉴权错乱。
  const resp = await fetch(`${API_BASE}${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
    credentials: "omit",
  });

  if (!resp.ok) {
    let msg = `Request failed (${resp.status})`;
    try {
      const data = await resp.json();
      if (data.error) msg = data.error;
      else if (data.message) msg = data.message;
    } catch {
      /* ignore */
    }
    // Login/register return 401 with JSON body; do not treat as session expiry.
    if (resp.status === 401 && !options?.noAuth) {
      useAuthStore.getState().logout();
      throw new Error("Session expired. Please sign in again.");
    }
    throw new Error(msg);
  }

  const text = await resp.text();
  if (!text) return undefined as T;
  return JSON.parse(text) as T;
}

export const api = {
  get: <T>(path: string) => request<T>("GET", path),
  post: <T>(path: string, body?: unknown) => request<T>("POST", path, body),
  patch: <T>(path: string, body?: unknown) => request<T>("PATCH", path, body),
  del: <T>(path: string) => request<T>("DELETE", path),

  // Auth
  login: (username: string, password: string) =>
    request<AuthResponse>("POST", "/api/auth/login", { username, password }, { noAuth: true }),
  register: (username: string, email: string, password: string) =>
    request<AuthResponse>("POST", "/api/auth/register", { username, email, password }, { noAuth: true }),
  getMe: () => request<UserInfo>("GET", "/api/auth/me"),
  updateEmail: (email: string) =>
    request<{ email: string }>("PUT", "/api/auth/me", { email }),
  updateUsername: (username: string) =>
    request<{ username: string }>("PUT", "/api/auth/me", { username }),
  updatePassword: (current_password: string, new_password: string) =>
    request<{ ok: boolean }>("PUT", "/api/auth/me/password", { current_password, new_password }),

  // Accounts
  getAccounts: () => request<Account[]>("GET", "/api/accounts"),
  startQRLogin: (device_name?: string) =>
    request<{ qr_id: string; qr_image: string; expire_at: number }>("POST", "/api/accounts/login/start", { device_name }),

  pollQRLogin: (qr_id: string) => request<QRStatus>("GET", `/api/accounts/login/poll?qr_id=${qr_id}`),
  deleteAccount: (id: number) => request<void>("DELETE", `/api/accounts/${id}`),

  // Search
  search: (q: string) => request<SearchResult[]>("GET", `/api/search?q=${encodeURIComponent(q)}`),

  // Subscriptions
  getSubscriptions: () => request<Subscription[]>("GET", "/api/subscriptions"),
  createSubscription: (book_id: string, alias: string) =>
    request<Subscription>("POST", "/api/subscriptions", { book_id, alias }),
  updateSubscription: (id: number, data: Partial<Subscription>) =>
    request<Subscription>("PATCH", `/api/subscriptions/${id}`, data),
  deleteSubscription: (id: number) => request<void>("DELETE", `/api/subscriptions/${id}`),
  getArticles: (id: number, offset?: number) =>
    request<Article[]>("GET", `/api/subscriptions/${id}/articles?offset=${offset || 0}`),
  refreshSubscription: (id: number) =>
    request<{ new_count: number }>("POST", `/api/subscriptions/${id}/refresh`),

  // Site config (SMTP)
  getConfig: () =>
    request<SiteConfig>("GET", "/api/config"),
  updateConfig: (data: Partial<SiteConfig>) =>
    request<{ ok: boolean }>("PUT", "/api/config", data),
};

// Types matching architecture.md
export interface Account {
  id: number;
  vid: number;
  nickname: string;
  avatar: string;
  status: "active" | "cooldown" | "dead";
  device_name?: string;
  cooldown_until?: number;
  last_ok_at?: number;
  last_err?: string;
  created_at: number;
}

export interface QRStatus {
  status: "pending" | "scanned" | "confirmed" | "expired" | "cancelled";
  credential?: {
    vid: number;
    nickname: string;
    avatar: string;
  };
}

export interface SearchResult {
  book_id: string;
  title: string;
  author: string;
  cover: string;
  mp_name?: string;
}

export interface Subscription {
  id: number;
  book_id: string;
  alias: string;
  mp_name: string;
  cover_url: string;
  fetch_interval_sec: number;
  /** -1 = no window; else minute of day 0..1439 (server local TZ) */
  fetch_window_start_min: number;
  fetch_window_end_min: number;
  last_fetch_at?: number;
  last_review_time?: number;
  created_at: number;
  disabled: boolean;
  feed_id: string;
  feed_url: string;
}

export interface UserInfo {
  id: number;
  username: string;
  email: string;
  is_admin: boolean;
  global_feed_url: string;
}

export interface AuthResponse extends UserInfo {
  token: string;
}

export interface SiteConfig {
  smtp_host: string;
  smtp_port: number;
  smtp_username: string;
  smtp_password: string;
  smtp_from: string;
  smtp_use_tls: boolean;
}

export interface Article {
  id: number;
  book_id: string;
  review_id: string;
  title: string;
  summary: string;
  content_html?: string;
  cover_url?: string;
  url?: string;
  publish_at: number;
  fetched_at: number;
  read_num: number;
  like_num: number;
}
