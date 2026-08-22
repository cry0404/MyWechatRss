import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, Loader2, MailCheck } from "lucide-react";
import { api } from "@/lib/api";
import { useAlertStore } from "@/stores/alertStore";
import { toUserMessage } from "@/lib/userMessage";

type SaveStatus = "idle" | "success";

export default function SettingsPage() {
  const qc = useQueryClient();
  const meQuery = useQuery({ queryKey: ["me"], queryFn: api.getMe });
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [username, setUsername] = useState("");
  const [usernameStatus, setUsernameStatus] = useState<SaveStatus>("idle");
  const [usernameError, setUsernameError] = useState("");
  const [email, setEmail] = useState("");
  const [emailStatus, setEmailStatus] = useState<SaveStatus>("idle");
  const [emailError, setEmailError] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [pwdStatus, setPwdStatus] = useState<SaveStatus>("idle");
  const [pwdError, setPwdError] = useState("");

  useEffect(() => {
    if (meQuery.data?.username !== undefined) setUsername(meQuery.data.username);
    if (meQuery.data?.email !== undefined) setEmail(meQuery.data.email);
  }, [meQuery.data?.username, meQuery.data?.email]);

  const updateUsername = useMutation({
    mutationFn: api.updateUsername,
    onSuccess: (data) => {
      qc.setQueryData(["me"], (old: unknown) =>
        old && typeof old === "object" ? { ...(old as object), username: data.username } : old
      );
      setUsernameStatus("success");
      setUsernameError("");
    },
    onError: (error) => setUsernameError(toUserMessage(error)),
  });

  const updateEmail = useMutation({
    mutationFn: api.updateEmail,
    onSuccess: (data) => {
      qc.setQueryData(["me"], (old: unknown) =>
        old && typeof old === "object" ? { ...(old as object), email: data.email } : old
      );
      setEmailStatus("success");
      setEmailError("");
    },
    onError: (error) => setEmailError(toUserMessage(error)),
  });

  const updatePassword = useMutation({
    mutationFn: () => api.updatePassword(currentPassword, newPassword),
    onSuccess: async () => {
      setPwdStatus("success");
      setPwdError("");
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      await qc.invalidateQueries({ queryKey: ["me"] });
    },
    onError: (error) => setPwdError(toUserMessage(error)),
  });

  const handleSaveUsername = () => {
    const value = username.trim();
    setUsernameStatus("idle");
    if (value.length < 3) {
      setUsernameError("用户名至少需要 3 个字符。");
      return;
    }
    setUsernameError("");
    updateUsername.mutate(value);
  };

  const handleSaveEmail = () => {
    const value = email.trim();
    setEmailStatus("idle");
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value)) {
      setEmailError("请输入有效的邮箱地址。");
      return;
    }
    setEmailError("");
    updateEmail.mutate(value);
  };

  const handleSavePassword = () => {
    setPwdStatus("idle");
    if (!currentPassword || !newPassword) {
      setPwdError("请输入当前密码和新密码。");
      return;
    }
    if (newPassword.length < 8) {
      setPwdError("新密码至少需要 8 个字符。");
      return;
    }
    if (newPassword !== confirmPassword) {
      setPwdError("两次输入的新密码不一致。");
      return;
    }
    setPwdError("");
    updatePassword.mutate();
  };

  const usernameDirty = meQuery.data ? username !== meQuery.data.username : false;
  const emailDirty = meQuery.data ? email !== meQuery.data.email : false;
  const pwdDirty = Boolean(currentPassword || newPassword || confirmPassword);

  return (
    <div className="page-enter max-w-xl mx-auto">
      <h1 className="text-3xl md:text-4xl font-heading mb-8 md:mb-10">设置</h1>

      <section className="pb-10 mb-10 border-b-2" style={{ borderColor: "var(--color-border-soft)" }}>
        <h2 className="text-2xl font-heading mb-6">账号</h2>
        {meQuery.isError && (
          <p className="mb-5 text-sm" style={{ color: "var(--color-danger)" }} role="alert">
            账号信息加载失败，请刷新后重试。
          </p>
        )}
        <div className="space-y-8">
          <FieldGroup label="用户名" inputId="settings-username" error={usernameError} success={usernameStatus === "success"}>
            <input
              id="settings-username"
              type="text"
              value={username}
              onChange={(event) => { setUsername(event.target.value); setUsernameError(""); setUsernameStatus("idle"); }}
              className="input-editorial flex-1 min-w-[200px]"
              autoComplete="username"
              disabled={updateUsername.isPending || meQuery.isLoading}
            />
            <SaveButton pending={updateUsername.isPending} disabled={!usernameDirty || meQuery.isLoading} onClick={handleSaveUsername} />
          </FieldGroup>

          <FieldGroup
            label="告警邮箱"
            inputId="settings-email"
            help="微信读书账号全部失效时，系统会通知到这个邮箱。"
            error={emailError}
            success={emailStatus === "success"}
          >
            <input
              id="settings-email"
              type="email"
              value={email}
              onChange={(event) => { setEmail(event.target.value); setEmailError(""); setEmailStatus("idle"); }}
              placeholder="name@example.com"
              className="input-editorial flex-1 min-w-[200px]"
              autoComplete="email"
              disabled={updateEmail.isPending || meQuery.isLoading}
            />
            <SaveButton pending={updateEmail.isPending} disabled={!emailDirty || meQuery.isLoading} onClick={handleSaveEmail} />
          </FieldGroup>

          <div id="password" className="scroll-mt-24 rounded-xl border-2 p-4 sm:p-5" style={{ borderColor: meQuery.data?.must_change_password ? "var(--color-warn)" : "var(--color-border-soft)" }}>
            <label htmlFor="current-password" className="block text-sm font-medium mb-1">登录密码</label>
            <p className="text-xs mb-4 leading-relaxed" style={{ color: "var(--color-ink-muted)" }}>
              {meQuery.data?.must_change_password ? "当前仍为初始默认密码，请立即修改。" : "至少 8 位，建议使用独立且难以猜测的密码。"}
            </p>
            <div className="space-y-3">
              <input id="current-password" type="password" value={currentPassword} onChange={(event) => { setCurrentPassword(event.target.value); setPwdError(""); setPwdStatus("idle"); }} placeholder="当前密码" className="input-editorial" autoComplete="current-password" disabled={updatePassword.isPending} />
              <input aria-label="新密码" type="password" value={newPassword} onChange={(event) => { setNewPassword(event.target.value); setPwdError(""); setPwdStatus("idle"); }} placeholder="新密码" className="input-editorial" autoComplete="new-password" disabled={updatePassword.isPending} />
              <input aria-label="确认新密码" type="password" value={confirmPassword} onChange={(event) => { setConfirmPassword(event.target.value); setPwdError(""); setPwdStatus("idle"); }} placeholder="再次输入新密码" className="input-editorial" autoComplete="new-password" disabled={updatePassword.isPending} />
              {pwdError && <p className="text-sm" style={{ color: "var(--color-danger)" }} role="alert">{pwdError}</p>}
              {pwdStatus === "success" && <p className="text-sm" style={{ color: "var(--color-success)" }} role="status">密码已更新</p>}
              <button type="button" onClick={handleSavePassword} disabled={!pwdDirty || updatePassword.isPending} className="btn-primary px-4 disabled:opacity-50">
                {updatePassword.isPending ? <Loader2 className="w-4 h-4 animate-spin" /> : "更新密码"}
              </button>
            </div>
          </div>
        </div>
      </section>

      {meQuery.data?.is_admin && (
        <details
          className="group pb-10 mb-10 border-b-2"
          style={{ borderColor: "var(--color-border-soft)" }}
          open={advancedOpen}
          onToggle={(event) => setAdvancedOpen(event.currentTarget.open)}
        >
          <summary className="flex cursor-pointer list-none items-center justify-between rounded-lg py-2">
            <span>
              <span className="block text-2xl font-heading">高级设置</span>
              <span className="mt-1 block text-xs" style={{ color: "var(--color-ink-muted)" }}>管理员专用 · 邮件告警</span>
            </span>
            <ChevronDown className="h-5 w-5 transition-transform group-open:rotate-180" aria-hidden />
          </summary>
          {advancedOpen && <SMTPConfigSection alertEmail={meQuery.data.email} />}
        </details>
      )}

      <section>
        <h2 className="text-2xl font-heading mb-3">关于</h2>
        <p className="text-lg leading-relaxed mb-4" style={{ color: "var(--color-ink-light)" }}>
          将微信读书公众号订阅转为 RSS，在阅读器中阅读。
        </p>
        <p className="text-xs font-mono" style={{ color: "var(--color-ink-muted)" }}>v0.1.0</p>
      </section>
    </div>
  );
}

function FieldGroup({ label, inputId, help, error, success, children }: { label: string; inputId: string; help?: string; error?: string; success?: boolean; children: React.ReactNode }) {
  return (
    <div>
      <label htmlFor={inputId} className="block text-sm font-medium mb-1">{label}</label>
      {help && <p className="text-xs mb-3 leading-relaxed" style={{ color: "var(--color-ink-muted)" }}>{help}</p>}
      <div className="flex gap-2 flex-wrap">{children}</div>
      {error && <p className="text-sm mt-2" style={{ color: "var(--color-danger)" }} role="alert">{error}</p>}
      {success && <p className="text-sm mt-2" style={{ color: "var(--color-success)" }} role="status">已保存</p>}
    </div>
  );
}

function SaveButton({ pending, disabled, onClick }: { pending: boolean; disabled: boolean; onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} disabled={disabled || pending} className="btn-primary min-w-[72px] px-4 disabled:opacity-50">
      {pending ? <Loader2 className="w-4 h-4 animate-spin" /> : "保存"}
    </button>
  );
}

function SMTPConfigSection({ alertEmail }: { alertEmail: string }) {
  const showAlert = useAlertStore((state) => state.show);
  const qc = useQueryClient();
  const configQuery = useQuery({ queryKey: ["site-config"], queryFn: api.getConfig });
  const [host, setHost] = useState("");
  const [port, setPort] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [from, setFrom] = useState("");
  const [useTLS, setUseTLS] = useState(false);
  const [status, setStatus] = useState<SaveStatus>("idle");
  const [error, setError] = useState("");

  useEffect(() => {
    if (!configQuery.data) return;
    setHost(configQuery.data.smtp_host || "");
    setPort(configQuery.data.smtp_port ? String(configQuery.data.smtp_port) : "");
    setUsername(configQuery.data.smtp_username || "");
    setPassword("");
    setFrom(configQuery.data.smtp_from || "");
    setUseTLS(configQuery.data.smtp_use_tls);
  }, [configQuery.data]);

  const updateMutation = useMutation({
    mutationFn: () => api.updateConfig({
      smtp_host: host.trim(),
      smtp_port: port.trim() ? Number.parseInt(port.trim(), 10) : 0,
      smtp_username: username.trim(),
      smtp_from: from.trim(),
      smtp_use_tls: useTLS,
      ...(password ? { smtp_password: password } : {}),
    }),
    onSuccess: async () => {
      setPassword("");
      setError("");
      setStatus("success");
      await qc.invalidateQueries({ queryKey: ["site-config"] });
    },
    onError: (mutationError) => setError(toUserMessage(mutationError)),
  });

  const testMutation = useMutation({
    mutationFn: api.testEmail,
    onSuccess: () => showAlert(`测试邮件已发送到 ${alertEmail}`, "success"),
    onError: (mutationError) => setError(toUserMessage(mutationError)),
  });

  const dirty = Boolean(
    configQuery.data && (
      host !== (configQuery.data.smtp_host || "") ||
      port !== String(configQuery.data.smtp_port || "") ||
      username !== (configQuery.data.smtp_username || "") ||
      password ||
      from !== (configQuery.data.smtp_from || "") ||
      useTLS !== configQuery.data.smtp_use_tls
    )
  );
  const configured = Boolean(configQuery.data?.smtp_host && configQuery.data.smtp_port > 0);

  const handleSave = () => {
    const parsedPort = Number.parseInt(port.trim(), 10);
    setStatus("idle");
    if (host.trim() && (!Number.isInteger(parsedPort) || parsedPort <= 0 || parsedPort > 65535)) {
      setError("SMTP 端口必须是 1–65535 的整数。");
      return;
    }
    setError("");
    updateMutation.mutate();
  };

  if (configQuery.isLoading) return <div className="flex justify-center py-10"><Loader2 className="h-5 w-5 animate-spin" /></div>;
  if (configQuery.isError) return <p className="mt-5 text-sm" style={{ color: "var(--color-danger)" }} role="alert">邮件配置加载失败，请重试。</p>;

  return (
    <div className="mt-6 space-y-5 rounded-xl border-2 p-4 sm:p-5" style={{ borderColor: "var(--color-border-soft)" }}>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h3 className="font-heading text-xl">邮件告警</h3>
          <p className="mt-1 text-xs leading-relaxed" style={{ color: "var(--color-ink-muted)" }}>
            配置保存在服务端；密码不会再次返回浏览器。
          </p>
        </div>
        <span className={`badge ${configured ? "badge-active" : "badge-dead"}`}>{configured ? "已配置" : "未配置"}</span>
      </div>

      <div className="space-y-4">
        <SMTPField id="smtp-host" label="SMTP 服务器" value={host} onChange={setHost} placeholder="smtp.example.com" />
        <SMTPField id="smtp-port" label="端口" value={port} onChange={setPort} placeholder="587" type="number" />
        <SMTPField id="smtp-username" label="用户名" value={username} onChange={setUsername} placeholder="sender@example.com" autoComplete="username" />
        <SMTPField id="smtp-password" label="密码或授权码" value={password} onChange={setPassword} placeholder={configQuery.data?.smtp_password_set ? "已配置，留空保持不变" : "输入密码或授权码"} type="password" autoComplete="new-password" />
        <SMTPField id="smtp-from" label="发件人" value={from} onChange={setFrom} placeholder="默认与用户名相同" />
        <label className="flex items-center gap-2 text-sm cursor-pointer">
          <input type="checkbox" checked={useTLS} onChange={(event) => { setUseTLS(event.target.checked); setStatus("idle"); }} className="h-4 w-4" />
          使用 TLS（465 端口通常需要）
        </label>
      </div>

      {error && <p className="text-sm" style={{ color: "var(--color-danger)" }} role="alert">{error}</p>}
      {status === "success" && <p className="text-sm" style={{ color: "var(--color-success)" }} role="status">配置已安全保存</p>}

      <div className="flex flex-wrap items-center gap-2">
        <button type="button" onClick={handleSave} disabled={!dirty || updateMutation.isPending} className="btn-primary px-4 disabled:opacity-50">
          {updateMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : "保存配置"}
        </button>
        <button type="button" onClick={() => { setError(""); testMutation.mutate(); }} disabled={!configured || dirty || testMutation.isPending || !alertEmail} className="btn-secondary px-4 disabled:opacity-50" title={dirty ? "请先保存配置" : undefined}>
          {testMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <MailCheck className="h-4 w-4" />}
          发送测试邮件
        </button>
      </div>
      <p className="text-xs" style={{ color: "var(--color-ink-muted)" }}>测试邮件将发送到 {alertEmail || "上方告警邮箱"}。</p>
    </div>
  );
}

function SMTPField({ id, label, value, onChange, placeholder, type = "text", autoComplete }: { id: string; label: string; value: string; onChange: (value: string) => void; placeholder: string; type?: string; autoComplete?: string }) {
  return (
    <div>
      <label htmlFor={id} className="block text-xs mb-1" style={{ color: "var(--color-ink-muted)" }}>{label}</label>
      <input id={id} type={type} value={value} onChange={(event) => onChange(event.target.value)} placeholder={placeholder} className="input-editorial w-full" autoComplete={autoComplete} />
    </div>
  );
}
