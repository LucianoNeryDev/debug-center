package sysinfo

import (
	"context"
	"os/exec"
	"strings"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

func hardware(ctx context.Context) (proto.Hardware, []proto.MemoryModule, []error) {
	hw := proto.Hardware{Manufacturer: "Apple"}
	if out, err := exec.CommandContext(ctx, "sysctl", "-n", "hw.model").Output(); err == nil {
		hw.Model = strings.TrimSpace(string(out))
	}
	if out, err := exec.CommandContext(ctx, "system_profiler", "SPDisplaysDataType").Output(); err == nil {
		for _, l := range strings.Split(string(out), "\n") {
			if t := strings.TrimSpace(l); strings.HasPrefix(t, "Chipset Model:") {
				hw.GPUs = append(hw.GPUs, strings.TrimSpace(strings.TrimPrefix(t, "Chipset Model:")))
			}
		}
	}
	return hw, nil, nil
}
