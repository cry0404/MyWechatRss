# WeChatRead RSS 客户端

自托管的微信读书公众号 RSS：部署在你自己的机器上，连接上游服务后即可在浏览器里管理账号与订阅，阅读器通过 RSS 链接收文章。

> 初始默认密码为 admin: changeme ， 请进入后及时更改，或尽量使用内网访问客户端
---

## 一键部署（推荐：Docker Compose）

```bash
git clone https://github.com/cry0404/MyWechatRss.git
cd MyWechatRss
cp .env.example .env
```

**配置：** 编辑项目根目录下的 `.env`（普通文本编辑器即可）。

1. **随机密钥**（各至少 16 个字符，且互不相同）。在终端执行两次下面命令，把输出分别填到 `APP_SECRET` 和 `JWT_SECRET`：

   ```bash
   openssl rand -hex 32
   ```

2. **上游服务**：向 Cry 索取并填写 `UPSTREAM_BASE_URL`、`UPSTREAM_API_KEY_ID`、`UPSTREAM_API_SECRET`。(baseurl 大概率为 https://wechat.cry4o4n0tfound.cc)

3. **你的网站地址**：`PUBLIC_BASE_URL` 填你实际用来访问本服务的地址（含 `http` 或 `https`，**不要**末尾斜杠）。例如本机试跑可用 `http://127.0.0.1:8081`；公网域名则填 `https://rss.example.com`。

4. **首次管理员**：保留或修改 `BOOTSTRAP_*`。数据库里还没有任何用户时，会用这组账号创建第一个管理员（仅一次）。**登录后请到「设置」里修改密码。**

然后启动：

```bash
docker compose up -d
```

浏览器打开你在 `PUBLIC_BASE_URL` 里填的地址即可。数据文件在项目下的 `data/` 目录（已挂载到容器内 `/data`）。

**升级镜像：**

```bash
docker compose pull && docker compose up -d
```

**官方镜像：** `ghcr.io/cry0404/MyWechatRss`

---

## 可选能力

| 需求 | 做法 |
|------|------|
| 换宿主机端口 | 在 `.env` 里增加 `HOST_PORT=9090`（或其它端口），再执行 `docker compose up -d`。 |
| 允许任何人注册 | `.env` 中设置 `ALLOW_REGISTER=true`。 |
| 账号全部失效时邮件提醒 | 配置 `SMTP_HOST`、`SMTP_PORT` 等；用户需在个人资料里填写邮箱。 |

---

## 从源码编译（可选）

仅在你需要改代码或无法使用镜像时使用。

```bash
git clone https://github.com/cry0404/MyWechatRss.git
cd MyWechatRss
cd web && npm ci && npm run build && cd ..
go build -o wechatread-client ./cmd/client
```

同样通过环境变量配置；本地可直接 `export` 或自建 `.env` 后用进程管理器加载。数据库路径等与 `dockerfile` 中说明一致时，可将 `DB_PATH` 指到持久化目录。

---

## 附录：环境变量一览

以下变量均可在 `.env` 中设置（Docker 与直接运行二进制均适用）。未列出的项有程序内默认值。

**必填（不填无法启动）：** `APP_SECRET`、`JWT_SECRET`、`UPSTREAM_BASE_URL`、`UPSTREAM_API_KEY_ID`、`UPSTREAM_API_SECRET`。

| 变量 | 说明 |
|------|------|
| `LISTEN_ADDR` | 监听地址，默认 `:8081`。 |
| `DB_PATH` | SQLite 路径；Docker 镜像内默认 `/data/app.db`（需挂载卷）。 |
| `PUBLIC_BASE_URL` | 对外访问根 URL，RSS 与邮件内链接依赖此项。 |
| `FEED_ID_SALT` | RSS feedId 加盐，默认 `wechatread-rss`。 |
| `ALLOW_REGISTER` | 是否开放注册，默认 `false`。 |
| `BOOTSTRAP_USERNAME` / `BOOTSTRAP_PASSWORD` / `BOOTSTRAP_EMAIL` | 空库时创建首个用户；`BOOTSTRAP_PASSWORD` 为空则不自动创建。 |
| `DEFAULT_DEVICE_NAME` | 微信读书侧设备名，默认 `wechatread-rss`。 |
| `SMTP_HOST`、`SMTP_PORT` | 二者齐全则启用发信；否则不发邮件。 |
| `SMTP_USERNAME`、`SMTP_PASSWORD`、`SMTP_FROM`、`SMTP_USE_TLS` | 邮件细节；`SMTP_FROM` 可空，回退为用户名。 |
