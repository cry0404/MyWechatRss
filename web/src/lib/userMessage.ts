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
    if (m.includes("no available weread account")) {
      return "当前没有可用的微信读书账号，请先绑定账号或等待冷却结束。";
    }
    if (m.includes("weread search/list rate limited")) {
      return "微信读书暂时限制了搜索请求，请稍后再试。";
    }
    if (m.includes("weread high-risk call deferred")) {
      return "账号凭证刚刚刷新，为降低风控风险，本次操作已延后，请稍后重试。";
    }
    if (
      m === "Failed to fetch" ||
      m.includes("NetworkError") ||
      m.includes("dial tcp") ||
      m.includes("connect: connection refused")
    ) {
      return "无法连接服务，请检查网络后重试。";
    }
    return m;
  }
  return "出错了，请稍后重试。";
}
