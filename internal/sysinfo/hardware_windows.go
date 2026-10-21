package sysinfo

import (
	"context"
	"fmt"
	"strings"

	"github.com/yusufpapurcu/wmi"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

type win32ComputerSystem struct {
	Manufacturer string
	Model        string
}

type win32BIOS struct {
	Manufacturer      string
	SMBIOSBIOSVersion string
	ReleaseDate       string
	SerialNumber      string
}

type win32BaseBoard struct {
	Manufacturer string
	Product      string
}

type win32VideoController struct {
	Name          string
	DriverVersion string
}

type win32PhysicalMemory struct {
	Capacity      uint64
	Speed         uint32
	Manufacturer  string
	PartNumber    string
	DeviceLocator string
}

type win32DiskDrive struct {
	Model         string
	Size          uint64
	InterfaceType string
	MediaType     string
	Status        string
}

func hardware(ctx context.Context) (proto.Hardware, []proto.MemoryModule, []error) {
	var hw proto.Hardware
	var errs []error
	q := func(dst any, query string) bool {
		if err := wmi.Query(query, dst); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", query, err))
			return false
		}
		return true
	}

	var cs []win32ComputerSystem
	if q(&cs, "SELECT Manufacturer, Model FROM Win32_ComputerSystem") && len(cs) > 0 {
		hw.Manufacturer, hw.Model = cs[0].Manufacturer, cs[0].Model
	}
	var bios []win32BIOS
	if q(&bios, "SELECT Manufacturer, SMBIOSBIOSVersion, ReleaseDate, SerialNumber FROM Win32_BIOS") && len(bios) > 0 {
		b := bios[0]
		hw.BIOSVendor, hw.BIOSVersion, hw.Serial = b.Manufacturer, b.SMBIOSBIOSVersion, b.SerialNumber
		if len(b.ReleaseDate) >= 8 { // CIM_DATETIME: yyyymmdd…
			hw.BIOSDate = b.ReleaseDate[:4] + "-" + b.ReleaseDate[4:6] + "-" + b.ReleaseDate[6:8]
		}
	}
	var bb []win32BaseBoard
	if q(&bb, "SELECT Manufacturer, Product FROM Win32_BaseBoard") && len(bb) > 0 {
		hw.BoardVendor, hw.BoardModel = bb[0].Manufacturer, bb[0].Product
	}
	var vc []win32VideoController
	if q(&vc, "SELECT Name, DriverVersion FROM Win32_VideoController") {
		for _, v := range vc {
			hw.GPUs = append(hw.GPUs, strings.TrimSpace(v.Name+" (driver "+v.DriverVersion+")"))
		}
	}
	var dd []win32DiskDrive
	if q(&dd, "SELECT Model, Size, InterfaceType, MediaType, Status FROM Win32_DiskDrive") {
		for _, d := range dd {
			hw.Disks = append(hw.Disks, proto.PhysicalDisk{
				Model: d.Model, Size: d.Size, Interface: d.InterfaceType, Media: d.MediaType, Status: d.Status,
			})
		}
	}
	var pm []win32PhysicalMemory
	var mods []proto.MemoryModule
	if q(&pm, "SELECT Capacity, Speed, Manufacturer, PartNumber, DeviceLocator FROM Win32_PhysicalMemory") {
		for _, m := range pm {
			mods = append(mods, proto.MemoryModule{
				Capacity: m.Capacity, SpeedMHz: m.Speed, Manufacturer: strings.TrimSpace(m.Manufacturer),
				PartNumber: strings.TrimSpace(m.PartNumber), Slot: m.DeviceLocator,
			})
		}
	}
	return hw, mods, errs
}
