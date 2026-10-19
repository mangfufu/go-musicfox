//go:build windows

package ui

import (
	"os"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var lyricsFocusUser = windows.NewLazySystemDLL("user32.dll")
var lyricsFocusKernel = windows.NewLazySystemDLL("kernel32.dll")

type lyricsHostSearch struct {
	pids map[uint32]int
	hwnd uintptr
	rank int
}

var lyricsHostCallback = windows.NewCallback(func(hwnd, data uintptr) uintptr {
	search := (*lyricsHostSearch)(unsafe.Pointer(data))
	var pid uint32
	lyricsFocusUser.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if rank, ok := search.pids[pid]; ok && rank < search.rank {
		if visible, _, _ := lyricsFocusUser.NewProc("IsWindowVisible").Call(hwnd); visible != 0 {
			search.hwnd, search.rank = hwnd, rank
		}
	}
	return 1
})

// A pseudoconsole's HWND is hidden; locate its terminal host through ancestors.
func focusLyricsTerminal() {
	hwnd, _, _ := lyricsFocusKernel.NewProc("GetConsoleWindow").Call()
	visible, _, _ := lyricsFocusUser.NewProc("IsWindowVisible").Call(hwnd)
	if visible == 0 {
		hwnd = 0
		snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
		if err == nil {
			defer windows.CloseHandle(snapshot)
			processes := make(map[uint32]windows.ProcessEntry32)
			entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
			for err := windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
				processes[entry.ProcessID] = entry
			}
			search := lyricsHostSearch{pids: make(map[uint32]int), rank: 100}
			pid := uint32(os.Getpid())
			for rank := 0; rank < 64 && pid != 0; rank++ {
				entry, ok := processes[pid]
				if !ok {
					break
				}
				switch strings.ToLower(windows.UTF16ToString(entry.ExeFile[:])) {
				case "windowsterminal.exe", "openconsole.exe", "conhost.exe", "cmd.exe", "powershell.exe", "pwsh.exe":
					search.pids[pid] = rank
				}
				if pid == entry.ParentProcessID {
					break
				}
				pid = entry.ParentProcessID
			}
			lyricsFocusUser.NewProc("EnumWindows").Call(lyricsHostCallback, uintptr(unsafe.Pointer(&search)))
			runtime.KeepAlive(&search)
			hwnd = search.hwnd
		}
	}
	if hwnd != 0 {
		if minimized, _, _ := lyricsFocusUser.NewProc("IsIconic").Call(hwnd); minimized != 0 {
			lyricsFocusUser.NewProc("ShowWindow").Call(hwnd, 9) // SW_RESTORE
		}
		lyricsFocusUser.NewProc("SetForegroundWindow").Call(hwnd)
	}
}
