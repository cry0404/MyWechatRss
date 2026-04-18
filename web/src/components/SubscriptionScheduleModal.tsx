import { useState, useEffect } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { api, type Subscription } from "@/lib/api";
import { ModalPortal } from "@/components/ModalPortal";
import { useAlertStore } from "@/stores/alertStore";
import { toUserMessage } from "@/lib/userMessage";
import {
  FETCH_INTERVAL_PRESETS,
  minToHM,
  hmToMin,
  HOURS,
  MINUTES,
} from "@/lib/schedule";

type Props = {
  subscription: Subscription;
  onClose: () => void;
};

/** 自定义时间选择器：小时 + 分钟下拉框，替代浏览器默认 type="time" */
function TimePicker({
  valueMin,
  onChange,
  label,
}: {
  valueMin: number;
  onChange: (min: number) => void;
  label: string;
}) {
  const { hour, minute } = minToHM(valueMin);

  return (
    <div className="flex flex-col gap-1">
      <span className="text-xs" style={{ color: "var(--color-ink-faint)" }}>
        {label}
      </span>
      <div className="flex items-center gap-1.5">
        <select
          value={hour}
          onChange={(e) => onChange(hmToMin(parseInt(e.target.value, 10), minute))}
          className="input-editorial !rounded-lg text-base px-2 py-1.5"
          style={{ width: "4.5rem" }}
        >
          {HOURS.map((h) => (
            <option key={h} value={h}>
              {String(h).padStart(2, "0")}
            </option>
          ))}
        </select>
        <span style={{ color: "var(--color-ink-muted)" }}>:</span>
        <select
          value={minute}
          onChange={(e) => onChange(hmToMin(hour, parseInt(e.target.value, 10)))}
          className="input-editorial !rounded-lg text-base px-2 py-1.5"
          style={{ width: "4.5rem" }}
        >
          {MINUTES.map((m) => (
            <option key={m} value={m}>
              {String(m).padStart(2, "0")}
            </option>
          ))}
        </select>
      </div>
    </div>
  );
}

export function SubscriptionScheduleModal({ subscription, onClose }: Props) {
  const qc = useQueryClient();
  const showAlert = useAlertStore((s) => s.show);
  const [preset, setPreset] = useState<string>(() => {
    const m = FETCH_INTERVAL_PRESETS.find((p) => p.sec === subscription.fetch_interval_sec);
    return m ? String(m.sec) : "custom";
  });
  const [customMin, setCustomMin] = useState(() =>
    Math.max(1, Math.round(subscription.fetch_interval_sec / 60))
  );
  const [useWindow, setUseWindow] = useState(
    subscription.fetch_window_start_min >= 0 && subscription.fetch_window_end_min >= 0
  );
  const [startMin, setStartMin] = useState(() =>
    subscription.fetch_window_start_min >= 0 ? subscription.fetch_window_start_min : 9 * 60
  );
  const [endMin, setEndMin] = useState(() =>
    subscription.fetch_window_end_min >= 0 ? subscription.fetch_window_end_min : 22 * 60
  );

  useEffect(() => {
    const m = FETCH_INTERVAL_PRESETS.find((p) => p.sec === subscription.fetch_interval_sec);
    setPreset(m ? String(m.sec) : "custom");
    setCustomMin(Math.max(1, Math.round(subscription.fetch_interval_sec / 60)));
    const w =
      subscription.fetch_window_start_min >= 0 && subscription.fetch_window_end_min >= 0;
    setUseWindow(w);
    if (subscription.fetch_window_start_min >= 0) {
      setStartMin(subscription.fetch_window_start_min);
    }
    if (subscription.fetch_window_end_min >= 0) {
      setEndMin(subscription.fetch_window_end_min);
    }
  }, [subscription]);

  const mutation = useMutation({
    mutationFn: () => {
      const sec =
        preset === "custom"
          ? Math.round(customMin) * 60
          : parseInt(preset, 10);
      const body: Partial<Subscription> = { fetch_interval_sec: sec };
      if (useWindow) {
        body.fetch_window_start_min = startMin;
        body.fetch_window_end_min = endMin;
      } else {
        body.fetch_window_start_min = -1;
        body.fetch_window_end_min = -1;
      }
      return api.updateSubscription(subscription.id, body);
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["subscriptions"] });
      qc.invalidateQueries({ queryKey: ["subscription", subscription.id] });
      onClose();
    },
    onError: (e) => showAlert(toUserMessage(e)),
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };

  return (
    <ModalPortal>
      <div className="fixed inset-0 z-[1000] flex items-center justify-center p-4">
        <div className="absolute inset-0 z-0 bg-black/40" onClick={onClose} aria-hidden />
        <form
          onSubmit={handleSubmit}
          className="relative z-[1] w-full max-w-md border-2 rounded-2xl bg-white p-6"
          style={{ borderColor: "var(--color-border)" }}
          onClick={(ev) => ev.stopPropagation()}
        >
          <h3 className="text-2xl font-heading mb-1">抓取计划</h3>
          <p className="text-sm mb-6 leading-relaxed" style={{ color: "var(--color-ink-muted)" }}>
            调度器按间隔检查是否到期；时段限制使用服务器本地时间（Docker 可设 TZ）。
          </p>

          <div className="space-y-5 mb-6">
            <div>
              <label className="block text-xs font-medium mb-1.5" style={{ color: "var(--color-ink-muted)" }}>
                自动抓取间隔
              </label>
              <div className="grid grid-cols-3 gap-2">
                {FETCH_INTERVAL_PRESETS.map((p) => {
                  const active = preset === String(p.sec);
                  return (
                    <button
                      key={p.sec}
                      type="button"
                      onClick={() => setPreset(String(p.sec))}
                      className="sketch-rounded-sm border-2 px-2 py-2 text-base transition-colors"
                      style={{
                        borderColor: active ? "var(--color-ink)" : "var(--color-border-soft)",
                        borderStyle: active ? "solid" : "dashed",
                        backgroundColor: active ? "var(--color-bg-muted)" : "transparent",
                        color: active ? "var(--color-ink)" : "var(--color-ink-muted)",
                        fontWeight: active ? 500 : 400,
                      }}
                    >
                      {p.label}
                    </button>
                  );
                })}
                <button
                  type="button"
                  onClick={() => setPreset("custom")}
                  className="sketch-rounded-sm border-2 px-2 py-2 text-base transition-colors"
                  style={{
                    borderColor: preset === "custom" ? "var(--color-ink)" : "var(--color-border-soft)",
                    borderStyle: preset === "custom" ? "solid" : "dashed",
                    backgroundColor: preset === "custom" ? "var(--color-bg-muted)" : "transparent",
                    color: preset === "custom" ? "var(--color-ink)" : "var(--color-ink-muted)",
                    fontWeight: preset === "custom" ? 500 : 400,
                  }}
                >
                  自定义
                </button>
              </div>
              {preset === "custom" && (
                <input
                  type="number"
                  min={1}
                  max={10080}
                  value={customMin}
                  onChange={(e) => setCustomMin(parseInt(e.target.value, 10) || 1)}
                  className="input-editorial !rounded-xl text-lg w-full mt-2"
                  placeholder="分钟"
                />
              )}
            </div>

            <div>
              <label className="flex items-center gap-2 text-base cursor-pointer">
                <input
                  type="checkbox"
                  checked={useWindow}
                  onChange={(e) => setUseWindow(e.target.checked)}
                  className="h-4 w-4 rounded border-2"
                  style={{ borderColor: "var(--color-border)" }}
                />
                <span style={{ color: "var(--color-ink)" }}>仅在以下时段自动抓取</span>
              </label>
              {useWindow && (
                <div className="flex flex-wrap items-center gap-4 mt-3">
                  <TimePicker valueMin={startMin} onChange={setStartMin} label="开始" />
                  <span style={{ color: "var(--color-ink-muted)" }} className="pt-4">至</span>
                  <TimePicker valueMin={endMin} onChange={setEndMin} label="结束" />
                </div>
              )}
            </div>
          </div>

          <div className="flex gap-2 justify-end">
            <button type="button" onClick={onClose} className="btn-secondary !rounded-xl px-4 py-2.5">
              取消
            </button>
            <button
              type="submit"
              disabled={mutation.isPending}
              className="btn-primary !rounded-xl px-4 py-2.5 min-w-[88px]"
            >
              {mutation.isPending ? <Loader2 className="h-5 w-5 animate-spin mx-auto" /> : "保存"}
            </button>
          </div>
        </form>
      </div>
    </ModalPortal>
  );
}
