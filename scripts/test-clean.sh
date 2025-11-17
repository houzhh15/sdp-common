#!/bin/bash
pkill -9 -f 'controller-example|ah-agent-example|ih-client-example|python3 -m http.server 9999' 2>/dev/null && echo "已清理测试进程"