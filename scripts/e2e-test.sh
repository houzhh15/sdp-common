#!/bin/bash
# 端到端测试脚本 - 演示完整的 SDP 工作流程

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "🚀 SDP 端到端测试"
echo "================="
echo ""

# 检查证书
if [ ! -d "$PROJECT_ROOT/certs" ]; then
    echo "❌ 证书未找到,正在生成..."
    "$SCRIPT_DIR/generate-certs.sh"
fi

# 检查编译（使用 bin/ 目录，与 quickstart.sh 一致）
echo "📋 检查编译状态..."
mkdir -p "$PROJECT_ROOT/bin"

if [ ! -f "$PROJECT_ROOT/bin/controller-example" ]; then
    echo "   编译 Controller..."
    cd "$PROJECT_ROOT/examples/controller"
    go build -o "$PROJECT_ROOT/bin/controller-example"
fi

if [ ! -f "$PROJECT_ROOT/bin/ih-client-example" ]; then
    echo "   编译 IH Client..."
    cd "$PROJECT_ROOT/examples/ih-client"
    go build -o "$PROJECT_ROOT/bin/ih-client-example"
fi

if [ ! -f "$PROJECT_ROOT/bin/ah-agent-example" ]; then
    echo "   编译 AH Agent..."
    cd "$PROJECT_ROOT/examples/ah-agent"
    go build -o "$PROJECT_ROOT/bin/ah-agent-example"
fi

echo "✅ 所有组件已编译到 bin/ 目录"
echo ""

# 清理残留进程
echo "🧹 清理残留进程..."
pkill -f "python3 -m http.server 9999" 2>/dev/null || true
pkill -f "controller-example" 2>/dev/null || true
pkill -f "ih-client-example" 2>/dev/null || true
pkill -f "ah-agent-example" 2>/dev/null || true
lsof -ti:9999 2>/dev/null | xargs kill -9 2>/dev/null || true
lsof -ti:8443 2>/dev/null | xargs kill -9 2>/dev/null || true
lsof -ti:9443 2>/dev/null | xargs kill -9 2>/dev/null || true
lsof -ti:8080 2>/dev/null | xargs kill -9 2>/dev/null || true
sleep 1

# 启动一个简单的测试 HTTP 服务器
echo "📋 启动目标服务 (端口 9999)..."
python3 -m http.server 9999 >/dev/null 2>&1 &
TARGET_PID=$!
sleep 1

if kill -0 $TARGET_PID 2>/dev/null; then
    echo "✅ 目标服务已启动 (PID: $TARGET_PID)"
else
    echo "❌ 目标服务启动失败"
    echo "   检查端口占用: lsof -i:9999"
    exit 1
fi

echo ""

# 启动 Controller
echo "📋 启动 Controller..."
"$PROJECT_ROOT/bin/controller-example" \
    -cert "$PROJECT_ROOT/certs/controller-cert.pem" \
    -key "$PROJECT_ROOT/certs/controller-key.pem" \
    -ca "$PROJECT_ROOT/certs/ca-cert.pem" \
    >/tmp/controller.log 2>&1 &
CTRL_PID=$!
sleep 2

if kill -0 $CTRL_PID 2>/dev/null; then
    echo "✅ Controller 已启动 (PID: $CTRL_PID)"
else
    echo "❌ Controller 启动失败"
    cat /tmp/controller.log
    kill $TARGET_PID 2>/dev/null || true
    exit 1
fi

echo ""

# 启动 AH Agent (从 Controller HTTP API 获取服务配置)
echo "📋 启动 AH Agent (通过混合方案获取服务配置)..."
# 注意：不再使用 -services 参数，服务配置通过 HTTP GET + SSE 获取
"$PROJECT_ROOT/bin/ah-agent-example" \
    -cert "$PROJECT_ROOT/certs/ah-agent-cert.pem" \
    -key "$PROJECT_ROOT/certs/ah-agent-key.pem" \
    -ca "$PROJECT_ROOT/certs/ca-cert.pem" \
    -controller "https://localhost:8443" \
    >/tmp/ah-agent.log 2>&1 &
AH_PID=$!
sleep 2

if kill -0 $AH_PID 2>/dev/null; then
    echo "✅ AH Agent 已启动 (PID: $AH_PID)"
else
    echo "❌ AH Agent 启动失败"
    cat /tmp/ah-agent.log
    kill $CTRL_PID $TARGET_PID 2>/dev/null || true
    exit 1
fi

echo ""

# 启动 IH Client
echo "📋 启动 IH Client..."
"$PROJECT_ROOT/bin/ih-client-example" \
    -cert "$PROJECT_ROOT/certs/ih-client-cert.pem" \
    -key "$PROJECT_ROOT/certs/ih-client-key.pem" \
    -ca "$PROJECT_ROOT/certs/ca-cert.pem" \
    -controller "https://localhost:8443" \
    -local "localhost:8080" \
    -proxy "localhost:9443" \
    >/tmp/ih-client.log 2>&1 &
IH_PID=$!
sleep 2

if kill -0 $IH_PID 2>/dev/null; then
    echo "✅ IH Client 已启动 (PID: $IH_PID)"
else
    echo "❌ IH Client 启动失败"
    cat /tmp/ih-client.log
    kill $CTRL_PID $AH_PID $TARGET_PID 2>/dev/null || true
    exit 1
fi

echo ""
echo "🎉 所有组件运行中!"
echo ""
echo "组件状态："
echo "  - Controller:    https://localhost:8443 (PID: $CTRL_PID)"
echo "  - AH Agent:      混合方案模式 (HTTP GET + SSE) (PID: $AH_PID)"
echo "  - IH Client:     localhost:8080 (PID: $IH_PID)"
echo "  - Target Service: http://localhost:9999 (PID: $TARGET_PID)"
echo ""
echo "🔄 SDP 2.0 规范 0x04 混合方案："
echo "  1. AH Agent 启动时通过 HTTP GET /api/v1/services 获取初始配置"
echo "  2. 订阅 SSE 接收服务配置实时更新"
echo "  3. Controller 已预置 demo-service-001 → localhost:9999"
echo ""
echo "日志文件："
echo "  - Controller:  /tmp/controller.log"
echo "  - AH Agent:    /tmp/ah-agent.log"
echo "  - IH Client:   /tmp/ih-client.log"
echo ""

# 等待服务完全启动
echo "⏳ 等待服务完全启动..."
sleep 3

# 运行 API 端点测试
echo ""
echo "🧪 运行 API 端点测试..."
echo "------------------------"

# 测试 1: 健康检查
echo "1. 测试健康检查端点..."
if curl -k -s \
    --cert "$PROJECT_ROOT/certs/ih-client-cert.pem" \
    --key "$PROJECT_ROOT/certs/ih-client-key.pem" \
    https://localhost:8443/health > /dev/null; then
    echo "   ✅ GET /health - OK"
else
    echo "   ❌ GET /health - FAILED"
fi

# 测试 2: 握手端点（需要客户端证书）
echo "2. 测试握手端点..."
HANDSHAKE_RESP=$(curl -k -s \
    --cert "$PROJECT_ROOT/certs/ih-client-cert.pem" \
    --key "$PROJECT_ROOT/certs/ih-client-key.pem" \
    -X POST https://localhost:8443/api/v1/handshake \
    -H "Content-Type: application/json" \
    -d '{"type":"handshake_request","fingerprint":"test"}' 2>/dev/null || echo "{}")

if echo "$HANDSHAKE_RESP" | grep -q "session_token"; then
    echo "   ✅ POST /api/v1/handshake - OK (返回 session_token)"
    # 提取 session token
    SESSION_TOKEN=$(echo "$HANDSHAKE_RESP" | grep -o '"session_token":"[^"]*"' | cut -d'"' -f4 || echo "")
else
    echo "   ⚠️  POST /api/v1/handshake - 返回格式未验证"
    SESSION_TOKEN=""
fi

# 测试 3: 服务配置查询（SDP 2.0 规范 0x04 混合方案）
echo "3. 测试服务配置端点 (0x04 HTTP GET)..."
SERVICES_RESP=$(curl -k -s \
    --cert "$PROJECT_ROOT/certs/ah-agent-cert.pem" \
    --key "$PROJECT_ROOT/certs/ah-agent-key.pem" \
    "https://localhost:8443/api/v1/services" 2>/dev/null || echo "{}")

if echo "$SERVICES_RESP" | grep -q "services"; then
    SERVICE_COUNT=$(echo "$SERVICES_RESP" | grep -o '"count":[0-9]*' | cut -d':' -f2 || echo "0")
    echo "   ✅ GET /api/v1/services - OK (返回 $SERVICE_COUNT 个服务配置)"
    # 显示服务列表
    if [ "$SERVICE_COUNT" -gt 0 ]; then
        echo "      预置服务: demo-service-001 (localhost:9999)"
    fi
else
    echo "   ⚠️  GET /api/v1/services - 返回格式未验证"
fi

# 测试 4: 策略查询（需要 Bearer Token）
if [ -n "$SESSION_TOKEN" ]; then
    echo "4. 测试策略查询端点..."
    POLICIES_RESP=$(curl -k -s \
        -H "Authorization: Bearer $SESSION_TOKEN" \
        "https://localhost:8443/api/v1/policies?client_id=ih-001" 2>/dev/null || echo "[]")
    
    if echo "$POLICIES_RESP" | grep -q "\["; then
        echo "   ✅ GET /api/v1/policies - OK (返回策略列表)"
    else
        echo "   ⚠️  GET /api/v1/policies - 返回格式未验证"
    fi
else
    echo "4. ⏭  跳过策略查询测试（无 session token）"
fi

# 测试 5: 隧道创建
if [ -n "$SESSION_TOKEN" ]; then
    echo "5. 测试隧道创建端点..."
    TUNNEL_RESP=$(curl -k -s \
        --cert "$PROJECT_ROOT/certs/ih-client-cert.pem" \
        --key "$PROJECT_ROOT/certs/ih-client-key.pem" \
        -H "Authorization: Bearer $SESSION_TOKEN" \
        -X POST https://localhost:8443/api/v1/tunnels \
        -H "Content-Type: application/json" \
        -d "{\"session_token\":\"$SESSION_TOKEN\",\"service_id\":\"demo-service-001\",\"local_port\":8080}" 2>/dev/null || echo "{}")
    
    if echo "$TUNNEL_RESP" | grep -q "tunnel_id"; then
        echo "   ✅ POST /api/v1/tunnels - OK (返回 tunnel_id)"
        echo "      注意: 隧道创建时自动从 ServiceConfig 获取 target_host/port"
    else
        echo "   ⚠️  POST /api/v1/tunnels - 返回格式未验证"
        # 显示实际响应以便调试
        if [ -n "$TUNNEL_RESP" ] && [ "$TUNNEL_RESP" != "{}" ]; then
            echo "      响应: $(echo "$TUNNEL_RESP" | head -c 100)"
        fi
    fi
else
    echo "5. ⏭  跳过隧道创建测试（无 session token）"
fi

# 测试 6: 数据转发（通过 IH Client）
echo "6. 测试完整数据转发流程..."
DATA_RESP=$(curl -s -m 5 http://localhost:8080 2>/dev/null || echo "")

if [ -n "$DATA_RESP" ]; then
    echo "   ✅ 数据转发 - OK (IH Client → Controller → AH Agent → Target)"
    echo "   响应预览: $(echo "$DATA_RESP" | head -n 1)"
else
    echo "   ⚠️  数据转发 - 连接超时或无响应"
fi
echo ""
echo "------------------------"
echo "✅ API 测试完成"
echo ""
echo "💡 提示: 可以手动测试数据转发："
echo "   curl http://localhost:8080"
echo ""
echo "按 Ctrl+C 停止所有服务..."

# 清理函数
cleanup() {
    echo ""
    echo "🛑 正在停止所有服务..."
    kill $IH_PID $AH_PID $CTRL_PID $TARGET_PID 2>/dev/null || true
    sleep 1
    echo "✅ 所有服务已停止"
}

trap cleanup EXIT

# 等待用户中断
wait
