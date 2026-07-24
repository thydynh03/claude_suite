package services

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type SchedulerService struct{}

func NewSchedulerService() *SchedulerService {
	return &SchedulerService{}
}

// SetCursorPos wraps Windows API SetCursorPos
func (s *SchedulerService) SetCursorPos(x, y int32) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("only supported on windows")
	}
	user32 := windows.NewLazySystemDLL("user32.dll")
	setCursorPos := user32.NewProc("SetCursorPos")
	r1, _, err := setCursorPos.Call(uintptr(x), uintptr(y))
	if r1 == 0 {
		return err
	}
	return nil
}

// ClickAt moves cursor and triggers left click
func (s *SchedulerService) ClickAt(x, y int32) error {
	if err := s.SetCursorPos(x, y); err != nil {
		return err
	}

	user32 := windows.NewLazySystemDLL("user32.dll")
	mouseEvent := user32.NewProc("mouse_event")

	const mouseEventFLeftDown = 0x0002
	const mouseEventFLeftUp = 0x0004

	mouseEvent.Call(uintptr(mouseEventFLeftDown), 0, 0, 0, 0)
	mouseEvent.Call(uintptr(mouseEventFLeftUp), 0, 0, 0, 0)

	return nil
}

// FindWindowByTitle returns HWND of target window
func (s *SchedulerService) FindWindowByTitle(title string) (uintptr, error) {
	if runtime.GOOS != "windows" {
		return 0, fmt.Errorf("only supported on windows")
	}
	user32 := windows.NewLazySystemDLL("user32.dll")
	findWindow := user32.NewProc("FindWindowW")

	tPtr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return 0, err
	}

	hwnd, _, _ := findWindow.Call(0, uintptr(unsafe.Pointer(tPtr)))
	if hwnd == 0 {
		return 0, fmt.Errorf("window '%s' not found", title)
	}
	return hwnd, nil
}
