/** Human-readable fetch interval */
export function formatInterval(sec: number): string {
  if (sec >= 86400) return `${Math.round(sec / 86400)} 天`;
  if (sec >= 3600) return `${Math.round(sec / 3600)} 小时`;
  return `${Math.round(sec / 60)} 分钟`;
}

/** Minute of day 0..1439 <-> [hour, minute] */
export function minToHM(min: number): { hour: number; minute: number } {
  const h = Math.floor(min / 60);
  const m = min % 60;
  return { hour: h, minute: m };
}

export function hmToMin(hour: number, minute: number): number {
  return Math.min(1439, Math.max(0, hour * 60 + minute));
}

export const HOURS = Array.from({ length: 24 }, (_, i) => i);
export const MINUTES = [0, 15, 30, 45];

/** 下拉选项：秒 */
export const FETCH_INTERVAL_PRESETS: { label: string; sec: number }[] = [
  { label: "6 小时", sec: 21600 },
  { label: "1 小时", sec: 3600 },
  { label: "2 小时", sec: 7200 },
  { label: "12 小时", sec: 43200 },
  { label: "24 小时", sec: 86400 },
  { label: "5 分钟", sec: 300 },
  { label: "15 分钟", sec: 900 },
  { label: "30 分钟", sec: 1800 },
];

function pad2(n: number) {
  return String(n).padStart(2, "0");
}

export function formatFetchWindowLine(startMin: number, endMin: number): string | null {
  if (startMin < 0 || endMin < 0) return null;
  const s = minToHM(startMin);
  const e = minToHM(endMin);
  return `${pad2(s.hour)}:${pad2(s.minute)}–${pad2(e.hour)}:${pad2(e.minute)}`;
}
