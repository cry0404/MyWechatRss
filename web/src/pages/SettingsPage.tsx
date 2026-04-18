import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { api } from "@/lib/api";
import { useAlertStore } from "@/stores/alertStore";
import { toUserMessage } from "@/lib/userMessage";

export default function SettingsPage() {
  const showAlert = useAlertStore((s) => s.show);
  const qc = useQueryClient();
  const meQuery = useQuery({
    queryKey: ["me"],
    queryFn: () => api.getMe(),
  });

  const [username, setUsername] = useState("");
  const [usernameStatus, setUsernameStatus] = useState<"idle" | "success">("idle");

  const [email, setEmail] = useState("");
  const [emailStatus, setEmailStatus] = useState<"idle" | "success">("idle");

  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [pwdStatus, setPwdStatus] = useState<"idle" | "success">("idle");

  useEffect(() => {
    if (meQuery.data?.username !== undefined) {
      setUsername(meQuery.data.username);
    }
    if (meQuery.data?.email !== undefined) {
      setEmail(meQuery.data.email);
    }
  }, [meQuery.data?.username, meQuery.data?.email]);

  const updateUsername = useMutation({
    mutationFn: (v: string) => api.updateUsername(v),
    onSuccess: (data) => {
      qc.setQueryData(["me"], (old: unknown) => {
        if (!old || typeof old !== "object") return old;
        return { ...(old as object), username: data.username };
      });
      setUsernameStatus("success");
    },
    onError: (err) => {
      showAlert(toUserMessage(err));
    },
  });

  const updateEmail = useMutation({
    mutationFn: (v: string) => api.updateEmail(v),
    onSuccess: (data) => {
      qc.setQueryData(["me"], (old: unknown) => {
        if (!old || typeof old !== "object") return old;
        return { ...(old as object), email: data.email };
      });
      setEmailStatus("success");
    },
    onError: (err) => {
      showAlert(toUserMessage(err));
    },
  });

  const updatePassword = useMutation({
    mutationFn: () => api.updatePassword(currentPassword, newPassword),
    onSuccess: () => {
      setPwdStatus("success");
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
    },
    onError: (err) => {
      showAlert(toUserMessage(err));
    },
  });

  const handleSaveUsername = () => {
    const v = username.trim();
    if (!v) {
      showAlert("Username is required.");
      return;
    }
    if (v.length < 3) {
      showAlert("Username must be at least 3 characters.");
      return;
    }
    setUsernameStatus("idle");
    updateUsername.mutate(v);
  };

  const handleSaveEmail = () => {
    const v = email.trim();
    if (!v) {
      showAlert("Email is required.");
      return;
    }
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v)) {
      showAlert("Invalid email address.");
      return;
    }
    setEmailStatus("idle");
    updateEmail.mutate(v);
  };

  const handleSavePassword = () => {
    setPwdStatus("idle");
    if (!currentPassword || !newPassword) {
      showAlert("Enter current password and new password.");
      return;
    }
    if (newPassword.length < 8) {
      showAlert("New password must be at least 8 characters.");
      return;
    }
    if (newPassword !== confirmPassword) {
      showAlert("New password and confirmation do not match.");
      return;
    }
    updatePassword.mutate();
  };

  const usernameDirty = meQuery.data ? username !== meQuery.data.username : false;
  const emailDirty = meQuery.data ? email !== meQuery.data.email : false;
  const pwdDirty = Boolean(currentPassword || newPassword || confirmPassword);

  return (
    <div className="page-enter max-w-xl mx-auto">
      <h1 className="text-4xl font-heading mb-10">设置</h1>

      <section className="pb-10 mb-10 border-b-2" style={{ borderColor: "var(--color-border-soft)" }}>
        <h2 className="text-2xl font-heading mb-6">账号</h2>
        <div className="space-y-6">
          <div>
            <p className="text-xs mb-1" style={{ color: "var(--color-ink-muted)" }}>
              用户名
            </p>
            <div className="flex gap-2 flex-wrap">
              <input
                type="text"
                value={username}
                onChange={(e) => {
                  setUsername(e.target.value);
                  setUsernameStatus("idle");
                }}
                placeholder="username"
                className="input-editorial flex-1 min-w-[200px]"
                disabled={updateUsername.isPending || meQuery.isLoading}
              />
              <button
                type="button"
                onClick={handleSaveUsername}
                disabled={!usernameDirty || updateUsername.isPending || meQuery.isLoading}
                className="btn-primary min-w-[72px] px-4"
              >
                {updateUsername.isPending ? <Loader2 className="w-4 h-4 animate-spin" /> : "保存"}
              </button>
            </div>
            {usernameStatus === "success" && (
              <p className="text-xs mt-2" style={{ color: "var(--color-success)" }}>
                已保存
              </p>
            )}
          </div>
          <div>
            <p className="text-xs mb-1" style={{ color: "var(--color-ink-muted)" }}>
              告警邮箱
            </p>
            <p className="text-xs mb-3 leading-relaxed" style={{ color: "var(--color-ink-muted)" }}>
              全部微信读书账号失效时会通知到此邮箱（需服务端配置 SMTP）。
            </p>
            <div className="flex gap-2 flex-wrap">
              <input
                type="email"
                value={email}
                onChange={(e) => {
                  setEmail(e.target.value);
                  setEmailStatus("idle");
                }}
                placeholder="name@example.com"
                className="input-editorial flex-1 min-w-[200px]"
                disabled={updateEmail.isPending || meQuery.isLoading}
              />
              <button
                type="button"
                onClick={handleSaveEmail}
                disabled={!emailDirty || updateEmail.isPending || meQuery.isLoading}
                className="btn-primary min-w-[72px] px-4"
              >
                {updateEmail.isPending ? <Loader2 className="w-4 h-4 animate-spin" /> : "保存"}
              </button>
            </div>
            {emailStatus === "success" && (
              <p className="text-xs mt-2" style={{ color: "var(--color-success)" }}>
                已保存
              </p>
            )}
          </div>

          <div>
            <p className="text-xs mb-1" style={{ color: "var(--color-ink-muted)" }}>
              登录密码
            </p>
            <p className="text-xs mb-3 leading-relaxed" style={{ color: "var(--color-ink-muted)" }}>
              修改用于登录本站的密码（至少 8 位）。
            </p>
            <div className="space-y-3 max-w-md">
              <input
                type="password"
                value={currentPassword}
                onChange={(e) => {
                  setCurrentPassword(e.target.value);
                  setPwdStatus("idle");
                }}
                placeholder="当前密码"
                className="input-editorial"
                autoComplete="current-password"
                disabled={updatePassword.isPending}
              />
              <input
                type="password"
                value={newPassword}
                onChange={(e) => {
                  setNewPassword(e.target.value);
                  setPwdStatus("idle");
                }}
                placeholder="新密码"
                className="input-editorial"
                autoComplete="new-password"
                disabled={updatePassword.isPending}
              />
              <input
                type="password"
                value={confirmPassword}
                onChange={(e) => {
                  setConfirmPassword(e.target.value);
                  setPwdStatus("idle");
                }}
                placeholder="确认新密码"
                className="input-editorial"
                autoComplete="new-password"
                disabled={updatePassword.isPending}
              />
              <button
                type="button"
                onClick={handleSavePassword}
                disabled={!pwdDirty || updatePassword.isPending}
                className="btn-primary px-4"
              >
                {updatePassword.isPending ? <Loader2 className="w-4 h-4 animate-spin" /> : "更新密码"}
              </button>
            </div>
            {pwdStatus === "success" && (
              <p className="text-xs mt-2" style={{ color: "var(--color-success)" }}>
                密码已更新
              </p>
            )}
          </div>
        </div>
      </section>

      <section className="pb-10 mb-10 border-b-2" style={{ borderColor: "var(--color-border-soft)" }}>
        <h2 className="text-2xl font-heading mb-6">邮件告警</h2>
        <SMTPConfigSection />
      </section>

      <section>
        <h2 className="text-2xl font-heading mb-3">关于</h2>
        <p className="text-lg leading-relaxed mb-4" style={{ color: "var(--color-ink-light)" }}>
          将微信读书公众号订阅转为 RSS，在阅读器中阅读。
        </p>
        <p className="text-xs font-mono" style={{ color: "var(--color-ink-muted)" }}>
          v0.1.0
        </p>
      </section>
    </div>
  );
}

function SMTPConfigSection() {
  const showAlert = useAlertStore((s) => s.show);
  const configQuery = useQuery({ queryKey: ["site-config"], queryFn: api.getConfig });
  const qc = useQueryClient();

  const [host, setHost] = useState("");
  const [port, setPort] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [from, setFrom] = useState("");
  const [useTLS, setUseTLS] = useState(false);
  const [status, setStatus] = useState<"idle" | "success">("idle");

  useEffect(() => {
    if (configQuery.data) {
      setHost(configQuery.data.smtp_host || "");
      setPort(configQuery.data.smtp_port ? String(configQuery.data.smtp_port) : "");
      setUsername(configQuery.data.smtp_username || "");
      setPassword(configQuery.data.smtp_password || "");
      setFrom(configQuery.data.smtp_from || "");
      setUseTLS(configQuery.data.smtp_use_tls);
    }
  }, [configQuery.data]);

  const updateMutation = useMutation({
    mutationFn: () =>
      api.updateConfig({
        smtp_host: host.trim() || undefined,
        smtp_port: port.trim() ? parseInt(port.trim(), 10) : undefined,
        smtp_username: username.trim() || undefined,
        smtp_password: password || undefined,
        smtp_from: from.trim() || undefined,
        smtp_use_tls: useTLS,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["site-config"] });
      setStatus("success");
    },
    onError: (err) => showAlert(toUserMessage(err)),
  });

  const handleSave = () => {
    const p = parseInt(port.trim(), 10);
    if (host.trim() && (Number.isNaN(p) || p <= 0)) {
      showAlert("SMTP 端口必须为正整数。");
      return;
    }
    setStatus("idle");
    updateMutation.mutate();
  };

  const dirty = Boolean(
    host !== (configQuery.data?.smtp_host || "") ||
      port !== String(configQuery.data?.smtp_port || "") ||
      username !== (configQuery.data?.smtp_username || "") ||
      password !== (configQuery.data?.smtp_password || "") ||
      from !== (configQuery.data?.smtp_from || "") ||
      useTLS !== (configQuery.data?.smtp_use_tls ?? false)
  );

  return (
    <div className="space-y-5 max-w-md">
      <p className="text-xs mb-2 leading-relaxed" style={{ color: "var(--color-ink-muted)" }}>
        全部微信读书账号失效时向告警邮箱发通知。留空则不发邮件。
      </p>

      <div className="space-y-3">
        <div>
          <p className="text-xs mb-1" style={{ color: "var(--color-ink-muted)" }}>SMTP 服务器</p>
          <input
            type="text"
            value={host}
            onChange={(e) => { setHost(e.target.value); setStatus("idle"); }}
            placeholder="smtp.example.com"
            className="input-editorial w-full"
            disabled={configQuery.isLoading}
          />
        </div>
        <div>
          <p className="text-xs mb-1" style={{ color: "var(--color-ink-muted)" }}>端口</p>
          <input
            type="number"
            value={port}
            onChange={(e) => { setPort(e.target.value); setStatus("idle"); }}
            placeholder="587"
            className="input-editorial w-full"
            disabled={configQuery.isLoading}
          />
        </div>
        <div>
          <p className="text-xs mb-1" style={{ color: "var(--color-ink-muted)" }}>用户名</p>
          <input
            type="text"
            value={username}
            onChange={(e) => { setUsername(e.target.value); setStatus("idle"); }}
            placeholder="sender@example.com"
            className="input-editorial w-full"
            disabled={configQuery.isLoading}
          />
        </div>
        <div>
          <p className="text-xs mb-1" style={{ color: "var(--color-ink-muted)" }}>密码</p>
          <input
            type="password"
            value={password}
            onChange={(e) => { setPassword(e.target.value); setStatus("idle"); }}
            placeholder="授权码或密码"
            className="input-editorial w-full"
            disabled={configQuery.isLoading}
          />
        </div>
        <div>
          <p className="text-xs mb-1" style={{ color: "var(--color-ink-muted)" }}>发件人</p>
          <input
            type="text"
            value={from}
            onChange={(e) => { setFrom(e.target.value); setStatus("idle"); }}
            placeholder="默认与用户名相同"
            className="input-editorial w-full"
            disabled={configQuery.isLoading}
          />
        </div>
        <label className="flex items-center gap-2 text-base cursor-pointer">
          <input
            type="checkbox"
            checked={useTLS}
            onChange={(e) => { setUseTLS(e.target.checked); setStatus("idle"); }}
            className="h-4 w-4 rounded border-2"
            style={{ borderColor: "var(--color-border)" }}
          />
          <span style={{ color: "var(--color-ink)" }}>使用 TLS（465 端口通常需要）</span>
        </label>
      </div>

      <div className="flex items-center gap-3 pt-1">
        <button
          type="button"
          onClick={handleSave}
          disabled={!dirty || updateMutation.isPending || configQuery.isLoading}
          className="btn-primary px-4"
        >
          {updateMutation.isPending ? <Loader2 className="w-4 h-4 animate-spin" /> : "保存"}
        </button>
        {status === "success" && (
          <p className="text-xs" style={{ color: "var(--color-success)" }}>已保存</p>
        )}
      </div>
    </div>
  );
}
