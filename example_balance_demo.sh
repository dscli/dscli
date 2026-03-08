#!/bin/bash

# 余额显示功能演示脚本
# 这个脚本演示了dscli chat命令如何显示用户余额和花费

echo "=== dscli 余额显示功能演示 ==="
echo ""
echo "功能说明："
echo "1. 在每次会话结束后，会显示会话花费"
echo "2. 同时显示当前余额"
echo "3. 当余额低于10元时，会显示提醒信息"
echo ""
echo "示例输出："
echo "⏱️  会话用时: 30.0s"
echo "💰  会话花费: CNY 4.50"
echo "💳  当前余额: CNY 95.50"
echo ""
echo "当余额较低时："
echo "⏱️  会话用时: 15.0s"
echo "💰  会话花费: CNY 0.80"
echo "💳  当前余额: CNY 5.20"
echo "⚠️  余额较低，请及时充值！"
echo ""
echo "使用方法："
echo "echo '你的问题' | dscli chat"
echo "或"
echo "dscli chat < 你的问题文件.txt"
echo ""
echo "注意：余额信息从DeepSeek API获取，需要有效的API密钥。"