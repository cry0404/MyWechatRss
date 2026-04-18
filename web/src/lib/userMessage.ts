/** Normalize API / network errors for user-facing Chinese messages. */
export function toUserMessage(err: unknown): string {
  if (err instanceof Error && err.message) {
    const m = err.message.trim();
    if (m === "invalid credentials") {
      return "用户名或密码错误。";
    }
    if (m === "当前密码不正确") {
      return "当前密码不正确。";
    }
    if (m === "Session expired. Please sign in again.") {
      return "会话已过期，请重新登录。";
    }
    return m;
  }
  return "出错了，请稍后重试。";
}
