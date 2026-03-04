# Backend

Go + Gin 后端（在线问答社区）。

## 目录结构

```text
backend/
  cmd/server/           # 入口
  internal/config/      # 配置
  internal/httpapi/     # 路由与接口
  scripts/              # 启停脚本
```

## 本地启动

```bash
cp .env.example .env
# 修改 DATABASE_URL/JWT_SECRET
./scripts/start.sh
```

## 初始化数据库表

```bash
psql "你的DATABASE_URL" -f ./migrations/0001_auth_init.sql
```

## 停止服务

```bash
./scripts/stop.sh
```

## 验证

```bash
curl http://127.0.0.1:18765/healthz
```

## Auth API

- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/logout` (Bearer token)
