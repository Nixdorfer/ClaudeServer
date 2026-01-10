//go:build !windows

package services

import "syscall"

func getHideWindowAttr() *syscall.SysProcAttr {
	return nil
}
