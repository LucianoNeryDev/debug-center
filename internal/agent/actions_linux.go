package agent

// catalogue holds Linux diagnostics and repairs, run with /bin/sh.
var catalogue = []action{
	def("journal-errors", "Errors from the journal since boot", "journalctl -p err -b --no-pager -n 200"),
	def("dmesg-errors", "Kernel errors and warnings", "dmesg --level=err,warn 2>/dev/null | tail -n 200"),
	def("failed-units", "Failed systemd units", "systemctl --failed --no-pager"),
	def("restart-service", "Restart a systemd unit", "systemctl restart {{1}} && systemctl status --no-pager {{1}}", params("unit")),
	def("disk-health", "Block devices and SMART health (needs smartmontools)",
		`lsblk -o NAME,SIZE,TYPE,MODEL,ROTA,MOUNTPOINTS; if command -v smartctl >/dev/null; then for d in $(lsblk -dno NAME -e7); do echo "== /dev/$d"; smartctl -H -A /dev/$d | sed -n '4,40p'; done; else echo "smartctl not installed"; fi`),
	def("hardware", "PCI and USB devices with kernel drivers", "lspci -k 2>/dev/null; echo; lsusb 2>/dev/null"),
	def("sensors", "Hardware sensors (lm-sensors)", "sensors 2>/dev/null || echo 'lm-sensors not installed'"),
	def("net-diag", "Addresses, routes, DNS and reachability",
		"ip -br addr; echo; ip route; echo; (resolvectl status 2>/dev/null || cat /etc/resolv.conf); echo; ping -c 3 -W 2 1.1.1.1"),
	def("flush-dns", "Flush the systemd-resolved cache", "resolvectl flush-caches && echo flushed"),
	def("updates", "Pending package updates",
		`if command -v apt >/dev/null; then apt list --upgradable 2>/dev/null; elif command -v dnf >/dev/null; then dnf -q check-update; elif command -v pacman >/dev/null; then pacman -Qu; else echo "unknown package manager"; fi`, slow),
	def("big-dirs", "Largest directories on the root filesystem", "du -xh / 2>/dev/null | sort -rh | head -n 30", slow),
	def("reboot", "Reboot in one minute", "shutdown -r +1 'Rebooting for maintenance'", dangerous),
	def("shutdown", "Shut down in one minute", "shutdown -h +1 'Shutting down for maintenance'", dangerous),
	def("cancel-shutdown", "Cancel a pending reboot or shutdown", "shutdown -c"),
}
