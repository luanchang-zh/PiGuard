#!/bin/sh
# Run on the Pi after enabling the official USB Ethernet gadget.
# This link is for direct PC/edge traffic, so it never needs ICS role switching.
set -eu
[ "$(id -u)" -eq 0 ] || { echo 'Run with sudo on the Raspberry Pi' >&2; exit 1; }
[ -d /sys/class/net/usb0 ] || { echo 'Enable USB gadget mode and reboot first' >&2; exit 1; }

backup_dir="/var/backups/piguard-usb/$(date +%Y%m%d-%H%M%S)"
mkdir -p "$backup_dir"
nmcli connection show > "$backup_dir/connections.txt"
systemctl is-enabled rpi-usb-gadget-ics.service > "$backup_dir/ics-enabled.txt" 2>/dev/null || true
if systemctl cat rpi-usb-gadget-ics.service >/dev/null 2>&1; then
  systemctl disable --now rpi-usb-gadget-ics.service
fi

# Keep the PC interface name stable across g_ether reloads and cold boots.
if [ -r /sys/module/g_ether/parameters/host_addr ] && [ -r /sys/module/g_ether/parameters/dev_addr ]; then
  host_mac=$(cat /sys/module/g_ether/parameters/host_addr)
  device_mac=$(cat /sys/module/g_ether/parameters/dev_addr)
  # Empty module parameters mean the kernel generated a random MAC. Use fixed
  # locally administered demo addresses on the next boot in that case.
  host_mac=${host_mac:-06:1e:b8:02:88:72}
  device_mac=${device_mac:-fe:e1:ca:4c:53:7c}
  mac_config=/etc/modprobe.d/piguard-usb-addresses.conf
  if [ -f "$mac_config" ]; then cp "$mac_config" "$backup_dir/"; fi
  printf 'options g_ether dev_addr=%s host_addr=%s\n' "$device_mac" "$host_mac" > "$mac_config"
  echo 'MAC settings saved; reboot once before configuring the PC USB interface.'
fi

for profile in 'USB Gadget (client)' 'USB Gadget (shared)'; do
  if nmcli connection show "$profile" >/dev/null 2>&1; then
    nmcli connection modify "$profile" connection.autoconnect no
    nmcli -w 5 connection down "$profile" >/dev/null 2>&1 || true
  fi
done

profile=piguard-usb-static
if ! nmcli connection show "$profile" >/dev/null 2>&1; then
  nmcli connection add type ethernet ifname usb0 con-name "$profile" \
    ipv4.method manual ipv4.addresses 10.12.194.1/28 ipv4.never-default yes \
    ipv4.ignore-auto-dns yes ipv6.method disabled connection.autoconnect yes
else
  nmcli connection modify "$profile" connection.interface-name usb0 \
    ipv4.method manual ipv4.addresses 10.12.194.1/28 ipv4.gateway '' \
    ipv4.never-default yes ipv4.ignore-auto-dns yes \
    ipv6.method disabled connection.autoconnect yes
fi
nmcli -w 15 connection up "$profile"
ip route get 10.12.194.2
echo "Pi USB address: 10.12.194.1; PC: 10.12.194.2/28; backup: $backup_dir"
