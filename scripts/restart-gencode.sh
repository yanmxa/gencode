#!/bin/bash
# GenCode 重启脚本
# 用于快速重新构建和启动 GenCode

set -e

echo "🔨 Building GenCode..."
npm run build

echo ""
echo "🚀 Starting GenCode..."
npm start
