// Package proto defines the wire types shared by the agent and the controller.
// The API is JSON over HTTP on the tailnet; streams use WebSocket.
package proto

import "time"

const (
	// Port is the tailnet TCP port the agent listens on.
	Port = 7420
	// HostnamePrefix marks agent nodes on the tailnet.
	HostnamePrefix = "dbc-"
	// APIVersion is bumped on incompatible wire changes.
	APIVersion = 1
)

// AgentInfo identifies a running agent.
type AgentInfo struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Hostname   string    `json:"hostname"` // tailnet hostname
	Version    string    `json:"version"`
	APIVersion int       `json:"api_version"`
	OS         string    `json:"os"`
	Arch       string    `json:"arch"`
	Elevated   bool      `json:"elevated"`
	Service    bool      `json:"service"` // installed as a service, not a foreground session
	StartedAt  time.Time `json:"started_at"`
}

// SystemInfo is the mostly static inventory of a machine.
type SystemInfo struct {
	Agent     AgentInfo  `json:"agent"`
	Collected time.Time  `json:"collected"`
	Host      HostInfo   `json:"host"`
	CPU       CPUInfo    `json:"cpu"`
	Memory    MemoryInfo `json:"memory"`
	Hardware  Hardware   `json:"hardware"`
	Volumes   []Volume   `json:"volumes"`
	NICs      []NIC      `json:"nics"`
	Users     []string   `json:"users,omitempty"`
	Warnings  []string   `json:"warnings,omitempty"`
}

type HostInfo struct {
	Hostname        string    `json:"hostname"`
	OS              string    `json:"os"`
	Platform        string    `json:"platform"`
	PlatformVersion string    `json:"platform_version"`
	KernelVersion   string    `json:"kernel_version"`
	KernelArch      string    `json:"kernel_arch"`
	Virtualization  string    `json:"virtualization,omitempty"`
	BootTime        time.Time `json:"boot_time"`
	UptimeSec       uint64    `json:"uptime_sec"`
}

type CPUInfo struct {
	Model         string  `json:"model"`
	Vendor        string  `json:"vendor"`
	PhysicalCores int     `json:"physical_cores"`
	LogicalCores  int     `json:"logical_cores"`
	MHz           float64 `json:"mhz"`
}

type MemoryInfo struct {
	Total     uint64         `json:"total"`
	SwapTotal uint64         `json:"swap_total"`
	Modules   []MemoryModule `json:"modules,omitempty"`
}

type MemoryModule struct {
	Capacity     uint64 `json:"capacity"`
	SpeedMHz     uint32 `json:"speed_mhz,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
	PartNumber   string `json:"part_number,omitempty"`
	Slot         string `json:"slot,omitempty"`
}

type Hardware struct {
	Manufacturer string         `json:"manufacturer,omitempty"`
	Model        string         `json:"model,omitempty"`
	Serial       string         `json:"serial,omitempty"`
	BIOSVendor   string         `json:"bios_vendor,omitempty"`
	BIOSVersion  string         `json:"bios_version,omitempty"`
	BIOSDate     string         `json:"bios_date,omitempty"`
	BoardVendor  string         `json:"board_vendor,omitempty"`
	BoardModel   string         `json:"board_model,omitempty"`
	GPUs         []string       `json:"gpus,omitempty"`
	Disks        []PhysicalDisk `json:"disks,omitempty"`
}

type PhysicalDisk struct {
	Model     string `json:"model"`
	Size      uint64 `json:"size"`
	Interface string `json:"interface,omitempty"`
	Media     string `json:"media,omitempty"`
	Status    string `json:"status,omitempty"`
}

type Volume struct {
	Device      string  `json:"device"`
	Mountpoint  string  `json:"mountpoint"`
	FSType      string  `json:"fstype"`
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"used_percent"`
}

type NIC struct {
	Name  string   `json:"name"`
	MAC   string   `json:"mac,omitempty"`
	MTU   int      `json:"mtu"`
	Up    bool     `json:"up"`
	Addrs []string `json:"addrs,omitempty"`
}

// Stats is one point-in-time sample of live load. Rates are per second since
// the previous sample in the same stream.
type Stats struct {
	Time       time.Time   `json:"time"`
	CPUPercent float64     `json:"cpu_percent"`
	PerCPU     []float64   `json:"per_cpu"`
	Load       []float64   `json:"load,omitempty"`
	MemTotal   uint64      `json:"mem_total"`
	MemUsed    uint64      `json:"mem_used"`
	SwapTotal  uint64      `json:"swap_total"`
	SwapUsed   uint64      `json:"swap_used"`
	Volumes    []VolumeUse `json:"volumes"`
	DiskIO     []IORate    `json:"disk_io"`
	NetIO      []IORate    `json:"net_io"`
	Temps      []Temp      `json:"temps,omitempty"`
	Processes  int         `json:"processes"`
	UptimeSec  uint64      `json:"uptime_sec"`
}

type VolumeUse struct {
	Mountpoint  string  `json:"mountpoint"`
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	UsedPercent float64 `json:"used_percent"`
}

// IORate is a read/write (disk) or recv/sent (network) rate in bytes/s.
type IORate struct {
	Name  string  `json:"name"`
	Read  float64 `json:"read"`
	Write float64 `json:"write"`
}

type Temp struct {
	Sensor  string  `json:"sensor"`
	Celsius float64 `json:"celsius"`
}

type Process struct {
	PID        int32   `json:"pid"`
	PPID       int32   `json:"ppid"`
	Name       string  `json:"name"`
	User       string  `json:"user,omitempty"`
	CPUPercent float64 `json:"cpu_percent"` // share of the whole machine
	RSS        uint64  `json:"rss"`
	Status     string  `json:"status,omitempty"`
	Cmdline    string  `json:"cmdline,omitempty"`
}

// Action is a named diagnostic or repair routine the agent knows how to run.
type Action struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Shell       string   `json:"shell"`
	Params      []string `json:"params,omitempty"`
	Dangerous   bool     `json:"dangerous,omitempty"` // disruptive: reboots, resets, deletes
	Slow        bool     `json:"slow,omitempty"`
}

// ExecRequest is the first message a client sends on /v1/exec. Exactly one of
// Command, Argv or Action is set.
type ExecRequest struct {
	Command    string   `json:"command,omitempty"` // run through Shell
	Shell      string   `json:"shell,omitempty"`   // powershell, cmd, sh, bash; empty = OS default
	Argv       []string `json:"argv,omitempty"`    // run directly, no shell
	Action     string   `json:"action,omitempty"`
	Args       []string `json:"args,omitempty"` // action parameters
	Dir        string   `json:"dir,omitempty"`
	TimeoutSec int      `json:"timeout_sec,omitempty"`
	Stdin      bool     `json:"stdin,omitempty"` // client will forward stdin
}

// Exec message types.
const (
	MsgStdout     = "stdout"
	MsgStderr     = "stderr"
	MsgExit       = "exit"
	MsgStdin      = "stdin"
	MsgStdinClose = "stdin_close"
	MsgKill       = "kill"
)

// ExecMessage flows both ways on /v1/exec after the request.
type ExecMessage struct {
	Type  string `json:"type"`
	Data  []byte `json:"data,omitempty"`
	Code  int    `json:"code,omitempty"`
	Error string `json:"error,omitempty"`
}

// FileEntry is one row of a directory listing.
type FileEntry struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	Mode    string    `json:"mode"`
	ModTime time.Time `json:"mod_time"`
	IsDir   bool      `json:"is_dir"`
}

// AuditEntry records one request to the agent.
type AuditEntry struct {
	Time    time.Time `json:"time"`
	Peer    string    `json:"peer"` // login name or tags
	Node    string    `json:"node"`
	Addr    string    `json:"addr"`
	Op      string    `json:"op"`
	Detail  string    `json:"detail,omitempty"`
	Allowed bool      `json:"allowed"`
}

// Error is the JSON body of non-2xx responses.
type Error struct {
	Error string `json:"error"`
}
