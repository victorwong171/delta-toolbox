//go:build windows

package main

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                         = windows.NewLazySystemDLL("user32.dll")
	procGetForegroundWindow        = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId   = user32.NewProc("GetWindowThreadProcessId")
	procGetWindowRect              = user32.NewProc("GetWindowRect")
	procMonitorFromWindow          = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW            = user32.NewProc("GetMonitorInfoW")
	procIsWindowVisible            = user32.NewProc("IsWindowVisible")
	procGetClassNameW              = user32.NewProc("GetClassNameW")

	kernel32                       = windows.NewLazySystemDLL("kernel32.dll")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
)

type RECT struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type MONITORINFO struct {
	CbSize    uint32
	RcMonitor RECT
	RcWork    RECT
	DwFlags   uint32
}

type WindowsPrioritizer struct{}

func NewOSPrioritizer() OSPrioritizer {
	return &WindowsPrioritizer{}
}

func initPlatform() {
	err := EnableDebugPrivilege()
	if err != nil {
		slog.Warn("Could not enable SeDebugPrivilege. Some games running as Administrator might not be modifiable.", "error", err)
	} else {
		slog.Info("Successfully enabled SeDebugPrivilege (elevated access).")
	}
}

func EnableDebugPrivilege() error {
	var token windows.Token
	h := windows.CurrentProcess()
	err := windows.OpenProcessToken(h, windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token)
	if err != nil {
		return fmt.Errorf("open process token: %w", err)
	}
	defer token.Close()

	var luid windows.LUID
	privNamePtr, err := windows.UTF16PtrFromString("SeDebugPrivilege")
	if err != nil {
		return err
	}
	err = windows.LookupPrivilegeValue(nil, privNamePtr, &luid)
	if err != nil {
		return fmt.Errorf("lookup privilege value: %w", err)
	}

	tp := windows.Tokenprivileges{
		PrivilegeCount: 1,
	}
	tp.Privileges[0].Luid = luid
	tp.Privileges[0].Attributes = windows.SE_PRIVILEGE_ENABLED

	err = windows.AdjustTokenPrivileges(token, false, &tp, 0, nil, nil)
	if err != nil {
		return fmt.Errorf("adjust token privileges: %w", err)
	}

	if errno := windows.GetLastError(); errno != nil && errno != syscall.Errno(0) {
		return errno
	}

	return nil
}

func GetProcessNameFallback(targetPID uint32) (string, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	err = windows.Process32First(snapshot, &entry)
	if err != nil {
		return "", err
	}

	for {
		if entry.ProcessID == targetPID {
			return windows.UTF16ToString(entry.ExeFile[:]), nil
		}
		err = windows.Process32Next(snapshot, &entry)
		if err != nil {
			break
		}
	}

	return "", fmt.Errorf("process not found in snapshot")
}

func (wp *WindowsPrioritizer) GetForegroundWindowInfo() (hwnd uintptr, pid uint32, err error) {
	r1, _, _ := procGetForegroundWindow.Call()
	if r1 == 0 {
		return 0, 0, fmt.Errorf("no foreground window")
	}

	var processID uint32
	_, _, _ = procGetWindowThreadProcessId.Call(r1, uintptr(unsafe.Pointer(&processID)))

	return r1, processID, nil
}

func (wp *WindowsPrioritizer) GetProcessName(pid uint32) (string, error) {
	// PROCESS_QUERY_LIMITED_INFORMATION (0x1000)
	h, err := windows.OpenProcess(0x1000, false, pid)
	if err != nil {
		name, fallbackErr := GetProcessNameFallback(pid)
		if fallbackErr == nil {
			slog.Debug("Resolved process name via Toolhelp fallback", "pid", pid, "name", name)
			return name, nil
		}
		return "", fmt.Errorf("open process: %w (fallback: %v)", err, fallbackErr)
	}
	defer windows.CloseHandle(h)

	var size uint32 = 1024
	buf := make([]uint16, size)

	r1, _, err := procQueryFullProcessImageNameW.Call(
		uintptr(h),
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if r1 == 0 {
		name, fallbackErr := GetProcessNameFallback(pid)
		if fallbackErr == nil {
			slog.Debug("Resolved process name via Toolhelp fallback after query fail", "pid", pid, "name", name)
			return name, nil
		}
		return "", fmt.Errorf("query process name: %w (fallback: %v)", err, fallbackErr)
	}

	path := windows.UTF16ToString(buf[:size])
	return filepath.Base(path), nil
}

func (wp *WindowsPrioritizer) IsFullscreen(hwnd uintptr, tolerance int) (bool, error) {
	// 1. Get window class name
	var classBuf [256]uint16
	ret, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&classBuf[0])), uintptr(len(classBuf)))
	if ret > 0 {
		className := windows.UTF16ToString(classBuf[:ret])
		// Ignore desktop, taskbar, start menu, and Windows shells
		if className == "Shell_TrayWnd" || className == "Progman" || className == "WorkerW" || className == "Windows.UI.Core.CoreWindow" {
			return false, nil
		}
	}

	// 2. Check visibility
	visible, _, _ := procIsWindowVisible.Call(hwnd)
	if visible == 0 {
		return false, nil
	}

	// 3. Get window dimensions
	var rect RECT
	ret, _, _ = procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	if ret == 0 {
		return false, fmt.Errorf("get window rect failed")
	}

	// 4. Get monitor info
	hMonitor, _, _ := procMonitorFromWindow.Call(hwnd, 2) // MONITOR_DEFAULTTONEAREST = 2
	if hMonitor == 0 {
		return false, fmt.Errorf("get monitor failed")
	}

	var mi MONITORINFO
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	ret, _, _ = procGetMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&mi)))
	if ret == 0 {
		return false, fmt.Errorf("get monitor info failed")
	}

	winWidth := rect.Right - rect.Left
	winHeight := rect.Bottom - rect.Top
	monWidth := mi.RcMonitor.Right - mi.RcMonitor.Left
	monHeight := mi.RcMonitor.Bottom - mi.RcMonitor.Top

	// Fuzzy matching with tolerance
	diffW := abs(winWidth - monWidth)
	diffH := abs(winHeight - monHeight)

	diffLeft := abs(rect.Left - mi.RcMonitor.Left)
	diffTop := abs(rect.Top - mi.RcMonitor.Top)

	slog.Debug("Checking window dimensions",
		"winWidth", winWidth, "monWidth", monWidth,
		"winHeight", winHeight, "monHeight", monHeight,
		"diffW", diffW, "diffH", diffH,
		"diffLeft", diffLeft, "diffTop", diffTop,
		"tolerance", tolerance,
	)

	// If the window covers the screen monitor area within tolerance bounds, it's fullscreen/borderless
	if diffW <= int32(tolerance) && diffH <= int32(tolerance) && diffLeft <= int32(tolerance) && diffTop <= int32(tolerance) {
		return true, nil
	}

	return false, nil
}

func (wp *WindowsPrioritizer) GetProcessPriority(pid uint32) (uint32, error) {
	h, err := windows.OpenProcess(0x1000, false, pid) // PROCESS_QUERY_LIMITED_INFORMATION
	if err != nil {
		return 0, fmt.Errorf("open process for query: %w", err)
	}
	defer windows.CloseHandle(h)

	priority, err := windows.GetPriorityClass(h)
	if err != nil {
		return 0, fmt.Errorf("get priority class: %w", err)
	}
	return priority, nil
}

func (wp *WindowsPrioritizer) SetProcessPriority(pid uint32, priority uint32) error {
	h, err := windows.OpenProcess(0x0200, false, pid) // PROCESS_SET_INFORMATION
	if err != nil {
		return fmt.Errorf("open process for set: %w", err)
	}
	defer windows.CloseHandle(h)

	err = windows.SetPriorityClass(h, priority)
	if err != nil {
		return fmt.Errorf("set priority class: %w", err)
	}
	return nil
}

func (wp *WindowsPrioritizer) GetPriorityClassValue(name string) uint32 {
	switch strings.ToLower(name) {
	case "abovenormal", "above_normal":
		return 0x00008000 // ABOVE_NORMAL_PRIORITY_CLASS
	case "high":
		return 0x00000080 // HIGH_PRIORITY_CLASS
	default:
		return 0x00000080 // Default to High
	}
}

func abs(n int32) int32 {
	if n < 0 {
		return -n
	}
	return n
}
