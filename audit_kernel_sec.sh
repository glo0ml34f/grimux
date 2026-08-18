#!/bin/bash

echo "== Kernel Configuration for IMA =="
grep 'IMA' /boot/config-$(uname -r) || echo "IMA not configured."

echo ""
echo "== IMA Runtime Status =="
if [ -d /sys/kernel/security/ima ]; then
  echo "IMA Policy:"
  cat /sys/kernel/security/ima/policy
  echo ""
  echo "IMA Runtime Measurements:"
  cat /sys/kernel/security/ima/ascii_runtime_measurements
else
  echo "IMA not active on this system."
fi

echo ""
echo "== Log Analysis for IMA =="
journalctl -k | grep -Ei 'ima|integrity' || echo "No IMA-related entries in kernel logs."

echo ""
echo "== SELinux Status =="
if command -v sestatus &> /dev/null; then
  sestatus
else
  echo "SELinux not available on this system."
fi

echo ""
echo "== AppArmor Status =="
if command -v aa-status &> /dev/null; then
  aa-status
else
  echo "AppArmor not available on this system."
fi

echo ""
echo "== Loaded Kernel Modules =="
lsmod | grep -E '(ima|selinux|apparmor)'

echo ""
echo "== Kernel Parameters =="
sysctl -a 2>/dev/null | grep -E '(kernel|security)'

echo ""
echo "== Lynis Availability Check =="
if command -v lynis &> /dev/null; then
  echo "Lynis is installed. You can audit the system with:"
  echo "sudo lynis audit system"
else
  echo "Lynis not installed. Install using your package manager, e.g., 'sudo apt install lynis'."
fi

echo ""
echo "== Auditd Check =="
if command -v auditctl &> /dev/null; then
  echo "Auditd is available. Use it to monitor detailed events."
else
  echo "Auditd not installed. Consider installing for comprehensive auditing."
fi
