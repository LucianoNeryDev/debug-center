//go:build !windows && !linux

package agent

// catalogue holds a minimal set for other Unix systems (macOS mostly).
var catalogue = []action{
	def("hardware", "Hardware overview", "system_profiler SPHardwareDataType SPStorageDataType 2>/dev/null || uname -a"),
	def("log-errors", "Errors from the unified log, last hour", "log show --last 1h --predicate 'messageType == error' --style compact 2>/dev/null | tail -n 200"),
	def("net-diag", "Interfaces, routes and reachability", "ifconfig; netstat -rn; ping -c 3 1.1.1.1"),
	def("reboot", "Reboot in one minute", "shutdown -r +1", dangerous),
	def("shutdown", "Shut down in one minute", "shutdown -h +1", dangerous),
}
