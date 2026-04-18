import { useState, useEffect, useRef } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { api, type Account } from "@/lib/api";
import { formatRelativeTime } from "@/lib/utils";
import { useQRLogin } from "@/hooks/useQRLogin";
import { ModalPortal } from "@/components/ModalPortal";
import { SafeImg } from "@/components/SafeImg";
import { useAlertStore } from "@/stores/alertStore";
import { toUserMessage } from "@/lib/userMessage";

function StatusText({ status }: { status: Account["status"] }) {
  const map: Record<Account["status"], string> = {
    active: "正常",
    cooldown: "冷却",
    dead: "失效",
  };
  return (
    <span className="text-xs tabular-nums" style={{ color: "var(--color-ink-muted)" }}>
      {map[status]}
    </span>
  );
}

function QRBindModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { phase, qrImage, start, reset, deviceName, setDeviceName } = useQRLogin(open);
  const queryClient = useQueryClient();

  useEffect(() => {
    if (phase === "success") {
      queryClient.invalidateQueries({ queryKey: ["accounts"] });
      const t = setTimeout(() => {
        onClose();
        reset();
      }, 1500);
      return () => clearTimeout(t);
    }
  }, [phase, onClose, reset, queryClient]);

  if (!open) return null;

  const phaseLabel: Record<string, string> = {
    loading: "正在生成二维码…",
    scanning: "等待扫码…",
    scanned: "扫码成功，等待确认…",
    success: "绑定成功",
    expired: "二维码已过期",
    error: "生成失败，请重试",
  };

  return (
    <ModalPortal>
      <div className="fixed inset-0 z-[1000] flex items-center justify-center p-4">
        <div className="absolute inset-0 z-0 bg-black/45" onClick={onClose} aria-hidden />
        <div className="relative z-[1] w-full max-w-sm border-2 bg-white p-8 rounded-xl" style={{ borderColor: "var(--color-border)" }}>
          <div className="flex justify-between items-start mb-6">
            <h3 className="text-2xl font-heading">绑定微信读书账号</h3>
            <button type="button" onClick={onClose} className="text-xs" style={{ color: "var(--color-ink-muted)" }}>
              关闭
            </button>
          </div>
          <div className="flex flex-col items-center">
            {phase === "idle" ? (
              <>
                <div className="w-full mb-5">
                  <label className="block text-xs mb-1" style={{ color: "var(--color-ink-muted)" }}>
                    设备名称（选填）
                  </label>
                  <input
                    type="text"
                    value={deviceName}
                    onChange={(e) => setDeviceName(e.target.value)}
                    placeholder="wechatread-rss"
                    className="w-full rounded border px-2 py-1.5 text-sm"
                    style={{
                      borderColor: "var(--color-border)",
                      backgroundColor: "var(--color-bg)",
                      color: "var(--color-ink)",
                    }}
                  />
                </div>
                <button type="button" onClick={start} className="btn-primary text-base px-6 py-2">
                  开始扫码
                </button>
              </>
            ) : (
              <>
                <div
                  className="relative w-52 h-52 flex items-center justify-center mb-4 border"
                  style={{ borderColor: "var(--color-border)", backgroundColor: "var(--color-bg)" }}
                >
                  {phase === "loading" && <Loader2 className="w-7 h-7 animate-spin" style={{ color: "var(--color-ink-muted)" }} />}
                  {(phase === "scanning" || phase === "scanned") && qrImage && (
                    <>
                      <img src={qrImage} alt="QR Code" className="w-full h-full object-contain p-2" />
                      <div className="absolute inset-x-2 qr-scan-line pointer-events-none">
                        <div className="w-full h-[2px]" style={{ backgroundColor: "var(--color-accent)", opacity: 0.5 }} />
                      </div>
                    </>
                  )}
                  {phase === "success" && (
                    <span className="text-sm font-medium" style={{ color: "var(--color-success)" }}>
                      完成
                    </span>
                  )}
                  {(phase === "expired" || phase === "error") && (
                    <div className="flex flex-col items-center gap-2">
                      <button type="button" onClick={start} className="btn-primary rounded text-xs px-3 py-1.5">
                        重新生成
                      </button>
                    </div>
                  )}
                </div>
                <p className="text-sm text-center" style={{ color: "var(--color-ink-muted)" }}>
                  {phaseLabel[phase] ?? ""}
                </p>
                <p className="text-xs text-center mt-5 leading-relaxed" style={{ color: "var(--color-ink-faint)" }}>
                  请使用微信读书 App 扫描上方二维码完成登录
                </p>
              </>
            )}
          </div>
        </div>
      </div>
    </ModalPortal>
  );
}

function EmptyState({ onBind }: { onBind: () => void }) {
  return (
    <div className="py-16 text-center page-enter max-w-md mx-auto rounded-xl border-2 px-6 py-10" style={{ borderColor: "var(--color-border-soft)" }}>
      <p className="text-xl font-heading mb-2" style={{ color: "var(--color-ink)" }}>
        尚未绑定账号
      </p>
      <p className="text-lg mb-6 leading-relaxed" style={{ color: "var(--color-ink-muted)" }}>
        绑定后可将公众号订阅同步为 RSS
      </p>
      <button type="button" onClick={onBind} className="btn-primary text-lg px-5 py-2.5">
        绑定账号
      </button>
    </div>
  );
}

function DeleteDialog({ account, onCancel, onConfirm }: { account: Account; onCancel: () => void; onConfirm: () => void }) {
  return (
    <ModalPortal>
      <div className="fixed inset-0 z-[1000] flex items-center justify-center p-4">
        <div className="absolute inset-0 z-0 bg-black/45" onClick={onCancel} aria-hidden />
        <div className="relative z-[1] w-full max-w-sm border-2 bg-white p-6 rounded-xl" style={{ borderColor: "var(--color-border)" }}>
        <h3 className="text-2xl font-heading mb-2">删除账号？</h3>
        <p className="text-sm mb-6" style={{ color: "var(--color-ink-muted)" }}>
          将移除 <strong>{account.nickname}</strong>（VID {account.vid}），不可恢复。
        </p>
        <div className="flex gap-2 justify-end">
          <button type="button" onClick={onCancel} className="btn-secondary rounded text-sm px-4 py-2">
            取消
          </button>
          <button
            type="button"
            onClick={onConfirm}
            className="rounded text-sm px-4 py-2 text-white font-medium"
            style={{ backgroundColor: "var(--color-danger)" }}
          >
            删除
          </button>
        </div>
        </div>
      </div>
    </ModalPortal>
  );
}

export default function AccountsPage() {
  const showAlert = useAlertStore((s) => s.show);
  const loadErrAlerted = useRef(false);
  const [qrOpen, setQrOpen] = useState(false);
  const [deleting, setDeleting] = useState<Account | null>(null);
  const queryClient = useQueryClient();

  const { data: accounts, isLoading, isError, error } = useQuery({ queryKey: ["accounts"], queryFn: api.getAccounts });

  useEffect(() => {
    if (isError && error && !loadErrAlerted.current) {
      loadErrAlerted.current = true;
      showAlert(toUserMessage(error));
    }
    if (!isError) loadErrAlerted.current = false;
  }, [isError, error, showAlert]);

  const deleteMutation = useMutation({
    mutationFn: (id: number) => api.deleteAccount(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["accounts"] });
      setDeleting(null);
    },
    onError: (err) => showAlert(toUserMessage(err)),
  });

  return (
    <div className="max-w-2xl mx-auto page-enter">
      <div className="flex items-start justify-between mb-8 gap-4">
        <div>
          <h1 className="text-4xl font-heading mb-1">账号</h1>
          <p className="text-lg" style={{ color: "var(--color-ink-muted)" }}>
            微信读书登录状态，用于拉取订阅与文章
          </p>
        </div>
        <button type="button" onClick={() => setQrOpen(true)} className="btn-primary text-lg px-5 py-2.5 shrink-0">
          绑定
        </button>
      </div>

      {isLoading && (
        <div className="flex items-center justify-center py-20">
          <Loader2 className="w-6 h-6 animate-spin" style={{ color: "var(--color-ink-muted)" }} />
        </div>
      )}

      {isError && (
        <p className="text-sm text-center py-16" style={{ color: "var(--color-ink-muted)" }}>
          加载失败，请刷新重试
        </p>
      )}

      {!isLoading && !isError && accounts && accounts.length === 0 && <EmptyState onBind={() => setQrOpen(true)} />}

      {!isLoading && !isError && accounts && accounts.length > 0 && (
        <div className="border-t-2" style={{ borderColor: "var(--color-border-soft)" }}>
          {accounts.map((account) => (
            <div
              key={account.id}
              className="list-row flex-wrap sm:flex-nowrap"
              style={{ paddingLeft: 0, paddingRight: 0 }}
            >
              <div className="shrink-0">
                {account.avatar ? (
                  <SafeImg src={account.avatar} alt="" className="w-11 h-11 rounded object-cover" />
                ) : (
                  <div className="w-11 h-11 rounded flex items-center justify-center text-xs font-medium" style={{ backgroundColor: "var(--color-bg-muted)" }}>
                    {account.nickname.slice(0, 1)}
                  </div>
                )}
              </div>
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 flex-wrap">
                  <span className="font-medium text-sm truncate">{account.nickname}</span>
                  <StatusText status={account.status} />
                </div>
                <p className="text-xs mt-0.5" style={{ color: "var(--color-ink-muted)" }}>
                  VID {account.vid}
                  {account.device_name ? ` · ${account.device_name}` : ""}
                  {account.last_ok_at ? ` · ${formatRelativeTime(account.last_ok_at)}` : ""}
                </p>
                {account.last_err && account.status === "dead" && (
                  <p className="text-xs mt-1 truncate" style={{ color: "var(--color-danger)" }}>
                    {account.last_err}
                  </p>
                )}
              </div>
              <button
                type="button"
                onClick={() => setDeleting(account)}
                className="text-sm px-3 py-1.5 rounded-md border-2 font-medium transition-colors hover:bg-red-50/80 shrink-0"
                style={{ borderColor: "var(--color-danger)", color: "var(--color-danger)" }}
              >
                删除
              </button>
            </div>
          ))}
        </div>
      )}

      <QRBindModal open={qrOpen} onClose={() => setQrOpen(false)} />
      {deleting && (
        <DeleteDialog account={deleting} onCancel={() => setDeleting(null)} onConfirm={() => deleting && deleteMutation.mutate(deleting.id)} />
      )}
    </div>
  );
}
