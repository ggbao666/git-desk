//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// hideConsoleWindow 让 git 子进程不弹出黑色控制台窗口。
// Wails 应用是 GUI 子系统程序，子进程若继承/新建控制台会闪屏。
func hideConsoleWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
}
