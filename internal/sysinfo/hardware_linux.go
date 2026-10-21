package sysinfo

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

func hardware(ctx context.Context) (proto.Hardware, []proto.MemoryModule, []error) {
	dmi := func(f string) string {
		b, err := os.ReadFile(filepath.Join("/sys/class/dmi/id", f))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	hw := proto.Hardware{
		Manufacturer: dmi("sys_vendor"),
		Model:        dmi("product_name"),
		Serial:       dmi("product_serial"), // readable by root only
		BIOSVendor:   dmi("bios_vendor"),
		BIOSVersion:  dmi("bios_version"),
		BIOSDate:     dmi("bios_date"),
		BoardVendor:  dmi("board_vendor"),
		BoardModel:   dmi("board_name"),
	}

	if out, err := exec.CommandContext(ctx, "lspci", "-mm").Output(); err == nil {
		sc := bufio.NewScanner(bytes.NewReader(out))
		for sc.Scan() {
			f := splitQuoted(sc.Text())
			if len(f) >= 4 && (strings.Contains(f[1], "VGA") || strings.Contains(f[1], "3D") || strings.Contains(f[1], "Display")) {
				hw.GPUs = append(hw.GPUs, f[2]+" "+f[3])
			}
		}
	}

	blocks, _ := os.ReadDir("/sys/block")
	for _, b := range blocks {
		name := b.Name()
		if skipDisk(name) || strings.HasPrefix(name, "md") {
			continue
		}
		base := filepath.Join("/sys/block", name)
		read := func(p string) string {
			v, _ := os.ReadFile(filepath.Join(base, p))
			return strings.TrimSpace(string(v))
		}
		sectors, _ := strconv.ParseUint(read("size"), 10, 64)
		if sectors == 0 {
			continue
		}
		d := proto.PhysicalDisk{Model: read("device/model"), Size: sectors * 512}
		if d.Model == "" {
			d.Model = name
		}
		switch read("queue/rotational") {
		case "0":
			d.Media = "SSD"
		case "1":
			d.Media = "HDD"
		}
		if strings.HasPrefix(name, "nvme") {
			d.Interface = "NVMe"
		}
		d.Status = read("device/state")
		hw.Disks = append(hw.Disks, d)
	}
	return hw, nil, nil
}

// splitQuoted splits an `lspci -mm` line into its quoted fields.
func splitQuoted(s string) []string {
	var out []string
	for {
		i := strings.IndexByte(s, '"')
		if i < 0 {
			return out
		}
		j := strings.IndexByte(s[i+1:], '"')
		if j < 0 {
			return out
		}
		out = append(out, s[i+1:i+1+j])
		s = s[i+j+2:]
	}
}
