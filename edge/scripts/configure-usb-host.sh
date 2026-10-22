#!/bin/sh
# Run on the Linux PC; pass the USB Ethernet interface shown by nmcli device.
set -eu
[ "$(id -u)" -eq 0 ] || { echo 'Run with sudo on the PC' >&2; exit 1; }
[ "$#" -eq 1 ] || { echo "Usage: sudo $0 USB_INTERFACE" >&2; exit 1; }
interface=$1
[ -d "/sys/class/net/$interface" ] || { echo 'Network interface does not exist' >&2; exit 1; }
profile=piguard-usb
if ! nmcli connection show "$profile" >/dev/null 2>&1; then
  nmcli connection add type ethernet ifname "$interface" con-name "$profile" \
    ipv4.method manual ipv4.addresses 10.12.194.2/28 ipv4.never-default yes \
    ipv4.ignore-auto-dns yes ipv6.method disabled connection.autoconnect yes
else
  nmcli connection modify "$profile" connection.interface-name "$interface" \
    ipv4.method manual ipv4.addresses 10.12.194.2/28 ipv4.gateway '' \
    ipv4.never-default yes ipv4.ignore-auto-dns yes ipv6.method disabled connection.autoconnect yes
fi
nmcli -w 15 connection up "$profile"
# Mihomo TUN may intercept even locally attached USB traffic. Its restart can
# rebuild this set; repeat this script after restarting the proxy if needed.
if command -v nft >/dev/null 2>&1 && nft list set inet mihomo inet4_local_address_set >/dev/null 2>&1; then
  nft add element inet mihomo inet4_local_address_set '{ 10.12.194.0/28 }'
fi
ip route get 10.12.194.1
echo 'PC USB address: 10.12.194.2; Raspberry Pi: 10.12.194.1/28'
