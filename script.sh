#!/bin/bash

echo "== System Information =="
uname -a
echo ""

echo "== Current User =="
whoami
echo ""

echo "== Hostname =="
hostname
echo ""

echo "== Network Information =="
ip addr show
echo ""

echo "== Processes =="
ps aux | head -n 10
echo ""

echo "== Open Ports =="
ss -tuln
echo ""

echo "== Environment Variables =="
printenv
echo ""

echo "== Disk Usage =="
df -h
echo ""

echo "== Memory Usage =="
free -h
echo ""

echo "== Sudo Privileges =="
sudo -l
echo ""

echo "== Cron Jobs =="
crontab -l 2>/dev/null || echo "No user-specific crontab."
echo ""
