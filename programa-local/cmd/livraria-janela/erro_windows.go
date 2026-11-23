//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// mostrarErro usa uma caixa de mensagem: o executável não tem console.
func mostrarErro(msg string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	box := user32.NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString("Livraria OT")
	m, _ := syscall.UTF16PtrFromString(msg)
	box.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x10)
}
