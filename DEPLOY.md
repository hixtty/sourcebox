# 部署笔记

当前线上实例：`https://asst.qzz.io`（Cloudflare Tunnel → 服务器 `127.0.0.1:8080`）。

## 服务器上的步骤

```bash
git clone https://github.com/hixtty/sourcebox.git ~/sourcebox
cd ~/sourcebox

printf 'BASE_URL=https://asst.qzz.io\nADMIN_TOKEN=%s\nCACHE_SECONDS=600\n' "$(head -c 48 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 32)" > .env

# 容器以 UID 10001 运行，挂载目录必须先交给它，否则 SQLite 会报
# "unable to open database file: out of memory (14)"
sudo mkdir -p data && sudo chown -R 10001:10001 data

sudo docker compose up -d --build
sudo docker logs sourcebox | tail -5   # 会打印 admin dashboard 地址
```

## 更新

```bash
cd ~/sourcebox && git pull && sudo docker compose up -d --build
```

## 取回后台路径

路径存在数据库 `kv.admin_path`，重启不变。日志里每次启动都会打印；也可以：

```bash
sudo docker exec sourcebox ./sourcebox -print-admin-path
```

## Cloudflare Tunnel

`/etc/cloudflared/config.yml` 里的 ingress 已经包含：

```yaml
- hostname: asst.qzz.io
  service: http://127.0.0.1:8080
```

改完配置后 `sudo systemctl restart cloudflared`。DNS 记录由 `cloudflared tunnel route dns <tunnel> asst.qzz.io` 管理，无需手填服务器 IP。

## 验证清单

```bash
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/health      # 200
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/            # 404
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/api/repos   # 404
curl -s "https://asst.qzz.io/<ADMIN_PATH>/source.json" | head -c 200
```
