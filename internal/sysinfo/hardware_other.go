//go:build !linux && !windows && !darwin

package sysinfo

import (
	"context"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

func hardware(context.Context) (proto.Hardware, []proto.MemoryModule, []error) {
	return proto.Hardware{}, nil, nil
}
