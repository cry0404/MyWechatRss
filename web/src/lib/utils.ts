const FEED_ONBOARDING_KEY = "wechatread:onboarding-feed-ready";

export function isFeedOnboardingReady(): boolean {
  return localStorage.getItem(FEED_ONBOARDING_KEY) === "1";
}

export function markFeedOnboardingReady(): void {
  localStorage.setItem(FEED_ONBOARDING_KEY, "1");
}

export function formatDate(ts: number): string {
  const d = new Date(ts * 1000);
  return d.toLocaleDateString("zh-CN", {
    year: "numeric",
    month: "long",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function formatRelativeTime(ts: number): string {
  const now = Date.now();
  const diff = Math.floor((now - ts * 1000) / 1000);

  if (diff < 60) return "刚刚";
  if (diff < 3600) return `${Math.floor(diff / 60)}分钟前`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}小时前`;
  if (diff < 604800) return `${Math.floor(diff / 86400)}天前`;
  return formatDate(ts);
}

export function formatFutureTime(ts: number): string {
  const diff = Math.ceil((ts * 1000 - Date.now()) / 1000);
  if (diff <= 60) return "1 分钟内";
  if (diff < 3600) return `${Math.ceil(diff / 60)} 分钟后`;
  if (diff < 86400) return `${Math.ceil(diff / 3600)} 小时后`;
  if (diff < 604800) return `${Math.ceil(diff / 86400)} 天后`;
  return formatDate(ts);
}

export function copyToClipboard(text: string): Promise<boolean> {
  return navigator.clipboard
    .writeText(text)
    .then(() => true)
    .catch(() => false);
}

export function truncateText(text: string, maxLen: number): string {
  if (text.length <= maxLen) return text;
  return text.slice(0, maxLen) + "...";
}
