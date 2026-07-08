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

const (
	CLSCTX_ALL              = 0x17
	COINIT_APARTMENTTHREADED = 0x2
)

var (
	ole32                = windows.NewLazySystemDLL("ole32.dll")
	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoUninitialize   = ole32.NewProc("CoUninitialize")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procPropVariantClear = ole32.NewProc("PropVariantClear")
)

type PROPERTYKEY struct {
	Fmtid windows.GUID
	Pid   uint32
}

type PROPVARIANT struct {
	Vt         uint16
	WReserved1 uint16
	WReserved2 uint16
	WReserved3 uint16
	PwszVal    *uint16
	Padding    [8]byte
}

func CoInitializeEx(reserved uintptr, dwCoInit uint32) uintptr {
	ret, _, _ := procCoInitializeEx.Call(reserved, uintptr(dwCoInit))
	return ret
}

func CoUninitialize() {
	_, _, _ = procCoUninitialize.Call()
}

func CoCreateInstance(rclsid *windows.GUID, pUnkOuter *byte, dwClsContext uint32, riid *windows.GUID, ppv *unsafe.Pointer) uintptr {
	ret, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(rclsid)),
		uintptr(unsafe.Pointer(pUnkOuter)),
		uintptr(dwClsContext),
		uintptr(unsafe.Pointer(riid)),
		uintptr(unsafe.Pointer(ppv)),
	)
	return ret
}

func PropVariantClear(pv *PROPVARIANT) {
	_, _, _ = procPropVariantClear.Call(uintptr(unsafe.Pointer(pv)))
}

// COM Interfaces structures

type IMMDeviceEnumerator struct {
	vtbl *immDeviceEnumeratorVtbl
}
type immDeviceEnumeratorVtbl struct {
	QueryInterface          uintptr
	AddRef                  uintptr
	Release                 uintptr
	EnumAudioEndpoints      uintptr
	GetDefaultAudioEndpoint uintptr
	GetDevice               uintptr
}

type IMMDeviceCollection struct {
	vtbl *immDeviceCollectionVtbl
}
type immDeviceCollectionVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	GetCount       uintptr
	Item           uintptr
}

type IMMDevice struct {
	vtbl *immDeviceVtbl
}
type immDeviceVtbl struct {
	QueryInterface     uintptr
	AddRef             uintptr
	Release            uintptr
	Activate           uintptr
	OpenPropertyStore  uintptr
	GetId              uintptr
	GetState           uintptr
}

type IPropertyStore struct {
	vtbl *iPropertyStoreVtbl
}
type iPropertyStoreVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	GetCount       uintptr
	GetAt          uintptr
	GetValue       uintptr
	SetValue       uintptr
	Commit         uintptr
}

type IPolicyConfig struct {
	vtbl *iPolicyConfigVtbl
}
type iPolicyConfigVtbl struct {
	QueryInterface        uintptr
	AddRef                uintptr
	Release               uintptr
	GetMixFormat          uintptr
	GetDeviceFormat       uintptr
	ResetDeviceFormat     uintptr
	SetDeviceFormat       uintptr
	GetProcessingPeriod   uintptr
	SetProcessingPeriod   uintptr
	GetShareMode          uintptr
	SetShareMode          uintptr
	GetPropertyValue      uintptr
	SetPropertyValue      uintptr
	SetDefaultEndpoint    uintptr
	SetEndpointVisibility uintptr
}

// Helpers calling COM methods

func (e *IMMDeviceEnumerator) Release() uint32 {
	ret, _, _ := syscall.SyscallN(e.vtbl.Release, uintptr(unsafe.Pointer(e)))
	return uint32(ret)
}

func (e *IMMDeviceEnumerator) EnumAudioEndpoints(dataFlow uint32, stateMask uint32, devices **IMMDeviceCollection) uintptr {
	ret, _, _ := syscall.SyscallN(e.vtbl.EnumAudioEndpoints,
		uintptr(unsafe.Pointer(e)),
		uintptr(dataFlow),
		uintptr(stateMask),
		uintptr(unsafe.Pointer(devices)),
	)
	return ret
}

func (c *IMMDeviceCollection) Release() uint32 {
	ret, _, _ := syscall.SyscallN(c.vtbl.Release, uintptr(unsafe.Pointer(c)))
	return uint32(ret)
}

func (c *IMMDeviceCollection) GetCount(count *uint32) uintptr {
	ret, _, _ := syscall.SyscallN(c.vtbl.GetCount,
		uintptr(unsafe.Pointer(c)),
		uintptr(unsafe.Pointer(count)),
	)
	return ret
}

func (c *IMMDeviceCollection) Item(index uint32, device **IMMDevice) uintptr {
	ret, _, _ := syscall.SyscallN(c.vtbl.Item,
		uintptr(unsafe.Pointer(c)),
		uintptr(index),
		uintptr(unsafe.Pointer(device)),
	)
	return ret
}

func (d *IMMDevice) Release() uint32 {
	ret, _, _ := syscall.SyscallN(d.vtbl.Release, uintptr(unsafe.Pointer(d)))
	return uint32(ret)
}

func (d *IMMDevice) OpenPropertyStore(stgmAccess uint32, properties **IPropertyStore) uintptr {
	ret, _, _ := syscall.SyscallN(d.vtbl.OpenPropertyStore,
		uintptr(unsafe.Pointer(d)),
		uintptr(stgmAccess),
		uintptr(unsafe.Pointer(properties)),
	)
	return ret
}

func (d *IMMDevice) GetId(id **uint16) uintptr {
	ret, _, _ := syscall.SyscallN(d.vtbl.GetId,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(id)),
	)
	return ret
}

func (p *IPropertyStore) Release() uint32 {
	ret, _, _ := syscall.SyscallN(p.vtbl.Release, uintptr(unsafe.Pointer(p)))
	return uint32(ret)
}

func (p *IPropertyStore) GetValue(key *PROPERTYKEY, pv *PROPVARIANT) uintptr {
	ret, _, _ := syscall.SyscallN(p.vtbl.GetValue,
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(key)),
		uintptr(unsafe.Pointer(pv)),
	)
	return ret
}

func (p *IPolicyConfig) Release() uint32 {
	ret, _, _ := syscall.SyscallN(p.vtbl.Release, uintptr(unsafe.Pointer(p)))
	return uint32(ret)
}

func (p *IPolicyConfig) SetDefaultEndpoint(deviceID *uint16, role uint32) uintptr {
	ret, _, _ := syscall.SyscallN(p.vtbl.SetDefaultEndpoint,
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(deviceID)),
		uintptr(role),
	)
	return ret
}

func getDeviceFriendlyName(device *IMMDevice) string {
	pkeyFriendlyNameFmt, _ := windows.GUIDFromString("{a45c254e-df1c-4efd-8020-67d146a850e0}")
	pkeyFriendlyName := PROPERTYKEY{
		Fmtid: pkeyFriendlyNameFmt,
		Pid:   2,
	}

	var propStore *IPropertyStore
	hrVal := device.OpenPropertyStore(0, &propStore)
	if hrVal != 0 {
		return ""
	}
	defer propStore.Release()

	var pv PROPVARIANT
	hrVal = propStore.GetValue(&pkeyFriendlyName, &pv)
	if hrVal == 0 {
		defer PropVariantClear(&pv)
		return windows.UTF16PtrToString(pv.PwszVal)
	}
	return ""
}

func getDeviceDescription(device *IMMDevice) string {
	pkeyDeviceDescFmt, _ := windows.GUIDFromString("{b3f8fa53-0004-438e-9003-51a46e139bfc}")
	pkeyDeviceDesc := PROPERTYKEY{
		Fmtid: pkeyDeviceDescFmt,
		Pid:   6,
	}

	var propStore *IPropertyStore
	hrVal := device.OpenPropertyStore(0, &propStore)
	if hrVal != 0 {
		return ""
	}
	defer propStore.Release()

	var pv PROPVARIANT
	hrVal = propStore.GetValue(&pkeyDeviceDesc, &pv)
	if hrVal == 0 {
		defer PropVariantClear(&pv)
		return windows.UTF16PtrToString(pv.PwszVal)
	}
	return ""
}

func (wp *WindowsPrioritizer) SwitchAudioDevice(name string) error {
	// Initialize COM
	hrVal := CoInitializeEx(0, COINIT_APARTMENTTHREADED)
	if hrVal != 0 && hrVal != 0x000401F0 && hrVal != 0x80010106 {
		return fmt.Errorf("CoInitializeEx failed: 0x%x", hrVal)
	}
	defer CoUninitialize()

	// Create IMMDeviceEnumerator
	clsidEnum, _ := windows.GUIDFromString("{BCDE0395-E52F-467C-8E3D-C4579291692E}")
	iidEnum, _ := windows.GUIDFromString("{A95664D2-9614-4F35-A746-DE8DB63617E6}")
	var enumerator *IMMDeviceEnumerator
	hrVal = CoCreateInstance(&clsidEnum, nil, CLSCTX_ALL, &iidEnum, (*unsafe.Pointer)(unsafe.Pointer(&enumerator)))
	if hrVal != 0 {
		return fmt.Errorf("CoCreateInstance(IMMDeviceEnumerator) failed: 0x%x", hrVal)
	}
	defer enumerator.Release()

	// Enum audio playback devices
	var collection *IMMDeviceCollection
	hrVal = enumerator.EnumAudioEndpoints(0, 1, &collection) // eRender = 0, DEVICE_STATE_ACTIVE = 1
	if hrVal != 0 {
		return fmt.Errorf("EnumAudioEndpoints failed: 0x%x", hrVal)
	}
	defer collection.Release()

	var count uint32
	hrVal = collection.GetCount(&count)
	if hrVal != 0 {
		return fmt.Errorf("GetCount failed: 0x%x", hrVal)
	}

	var targetDeviceID *uint16
	var targetDeviceName string
	nameLower := strings.ToLower(name)

	for i := uint32(0); i < count; i++ {
		var device *IMMDevice
		hrVal = collection.Item(i, &device)
		if hrVal != 0 {
			continue
		}
		defer device.Release()

		var idStr *uint16
		hrVal = device.GetId(&idStr)
		if hrVal != 0 {
			continue
		}

		friendlyName := getDeviceFriendlyName(device)
		description := getDeviceDescription(device)
		if strings.Contains(strings.ToLower(friendlyName), nameLower) || strings.Contains(strings.ToLower(description), nameLower) {
			targetDeviceID = idStr
			targetDeviceName = friendlyName
			break
		}
	}

	if targetDeviceID == nil {
		return fmt.Errorf("no audio device matching name: %s", name)
	}

	// Create IPolicyConfig
	clsidPolicy, _ := windows.GUIDFromString("{870AF99C-171D-4F9E-AF0D-E63DF40C2BC9}")
	iidPolicy, _ := windows.GUIDFromString("{F8679F50-850A-41CF-9C72-430F290290C8}")
	var policyConfig *IPolicyConfig
	hrVal = CoCreateInstance(&clsidPolicy, nil, CLSCTX_ALL, &iidPolicy, (*unsafe.Pointer)(unsafe.Pointer(&policyConfig)))
	if hrVal != 0 {
		return fmt.Errorf("CoCreateInstance(IPolicyConfig) failed: 0x%x", hrVal)
	}
	defer policyConfig.Release()

	// Set default endpoint for Console (0), Multimedia (1), Communications (2)
	var lastErr error
	for _, role := range []uint32{0, 1, 2} {
		hrVal = policyConfig.SetDefaultEndpoint(targetDeviceID, role)
		if hrVal != 0 {
			lastErr = fmt.Errorf("SetDefaultEndpoint role %d failed: 0x%x", role, hrVal)
		}
	}

	if lastErr != nil {
		return lastErr
	}

	slog.Info("Successfully switched default audio playback device", "name", targetDeviceName)
	return nil
}
