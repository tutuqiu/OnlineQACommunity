#!/usr/bin/env bash
set -euo pipefail

# 说明：该脚本用于验证现有 Auth API 主流程是否可用。
BASE_URL="${BASE_URL:-http://127.0.0.1:18765}"

# 说明：为避免重复执行时与历史数据冲突，注册用户使用时间戳后缀。
SUFFIX="$(date +%s)"
EMAIL="api-test-${SUFFIX}@example.com"
USERNAME="api_test_${SUFFIX}"
PASSWORD="TestPass123!"

if ! command -v curl >/dev/null 2>&1; then
  echo "[失败] 未找到 curl，请先安装 curl。"
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "[失败] 未找到 jq，请先安装 jq。"
  exit 1
fi

TMP_BODY="$(mktemp)"
trap 'rm -f "$TMP_BODY"' EXIT

request_json() {
  local method="$1"
  local path="$2"
  local payload="${3:-}"
  local auth_header="${4:-}"

  if [[ -n "$auth_header" ]]; then
    if [[ -n "$payload" ]]; then
      curl -sS -o "$TMP_BODY" -w "%{http_code}" \
        -X "$method" "${BASE_URL}${path}" \
        -H "Content-Type: application/json" \
        -H "Authorization: ${auth_header}" \
        -d "$payload"
    else
      curl -sS -o "$TMP_BODY" -w "%{http_code}" \
        -X "$method" "${BASE_URL}${path}" \
        -H "Authorization: ${auth_header}"
    fi
  else
    if [[ -n "$payload" ]]; then
      curl -sS -o "$TMP_BODY" -w "%{http_code}" \
        -X "$method" "${BASE_URL}${path}" \
        -H "Content-Type: application/json" \
        -d "$payload"
    else
      curl -sS -o "$TMP_BODY" -w "%{http_code}" \
        -X "$method" "${BASE_URL}${path}"
    fi
  fi
}

assert_status() {
  local actual="$1"
  local expected="$2"
  local action="$3"

  if [[ "$actual" != "$expected" ]]; then
    echo "[失败] ${action} 状态码不符合预期，期望=${expected}，实际=${actual}"
    echo "[失败] 响应体：$(cat "$TMP_BODY")"
    exit 1
  fi
  echo "[通过] ${action}"
}

echo "[信息] 开始测试，服务地址：${BASE_URL}"

HEALTH_CODE="$(request_json GET /healthz)"
assert_status "$HEALTH_CODE" "200" "健康检查 /healthz"

REGISTER_PAYLOAD="$(jq -cn --arg email "$EMAIL" --arg username "$USERNAME" --arg password "$PASSWORD" '{email:$email,username:$username,password:$password}')"
REGISTER_CODE="$(request_json POST /api/v1/auth/register "$REGISTER_PAYLOAD")"
assert_status "$REGISTER_CODE" "201" "注册接口 /api/v1/auth/register"

LOGIN_PAYLOAD="$(jq -cn --arg email "$EMAIL" --arg password "$PASSWORD" '{email:$email,password:$password}')"
LOGIN_CODE="$(request_json POST /api/v1/auth/login "$LOGIN_PAYLOAD")"
assert_status "$LOGIN_CODE" "200" "登录接口 /api/v1/auth/login"

ACCESS_TOKEN="$(jq -r '.access_token // empty' "$TMP_BODY")"
REFRESH_TOKEN="$(jq -r '.refresh_token // empty' "$TMP_BODY")"
if [[ -z "$ACCESS_TOKEN" || -z "$REFRESH_TOKEN" ]]; then
  echo "[失败] 登录响应缺少 access_token 或 refresh_token"
  echo "[失败] 响应体：$(cat "$TMP_BODY")"
  exit 1
fi

echo "[通过] 登录响应包含令牌字段"

REFRESH_PAYLOAD="$(jq -cn --arg refresh_token "$REFRESH_TOKEN" '{refresh_token:$refresh_token}')"
REFRESH_CODE="$(request_json POST /api/v1/auth/refresh "$REFRESH_PAYLOAD")"
assert_status "$REFRESH_CODE" "200" "刷新接口 /api/v1/auth/refresh"

NEW_ACCESS_TOKEN="$(jq -r '.access_token // empty' "$TMP_BODY")"
if [[ -z "$NEW_ACCESS_TOKEN" ]]; then
  echo "[失败] 刷新响应缺少新的 access_token"
  echo "[失败] 响应体：$(cat "$TMP_BODY")"
  exit 1
fi

echo "[通过] 刷新响应包含新的访问令牌"

LOGOUT_CODE="$(request_json POST /api/v1/auth/logout "" "Bearer ${NEW_ACCESS_TOKEN}")"
assert_status "$LOGOUT_CODE" "200" "登出接口 /api/v1/auth/logout"

LOGOUT_AGAIN_CODE="$(request_json POST /api/v1/auth/logout "" "Bearer ${NEW_ACCESS_TOKEN}")"
assert_status "$LOGOUT_AGAIN_CODE" "401" "登出后令牌拦截校验 /api/v1/auth/logout"

echo "[通过] 所有 API 用例执行完成"
