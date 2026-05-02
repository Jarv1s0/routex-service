//go:build windows
// +build windows

package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	user32          = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW = user32.NewProc("MessageBoxW")
	mbOk            = uintptr(0)
	mbIconError     = uintptr(0x10)
	messageBoxFlags = mbOk | mbIconError
	messageBoxTitle = "RouteX Runner"
	paramFileName   = "param.txt"
)

func main() {
	if err := run(); err != nil {
		showError("Error: " + err.Error())
	}
}

func run() error {
	target, targetArgs, err := resolveTargetArgs(os.Args)
	if err != nil {
		return err
	}

	cmd := exec.Command(target, targetArgs...)
	if err := cmd.Start(); err != nil {
		errorMessage := "Failed to start program\n" + target + "\n" + err.Error() + "\n请尝试以管理员权限启动软件"
		return errors.New(errorMessage)
	}

	return nil
}

func resolveTargetArgs(args []string) (string, []string, error) {
	if len(args) < 2 {
		return "", nil, errors.New("invalid arguments")
	}

	target := args[1]
	if len(args) > 2 {
		return target, append([]string(nil), args[2:]...), nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return "", nil, err
	}

	exeDir := filepath.Dir(exePath)
	paramPath := filepath.Join(exeDir, paramFileName)
	targetArgs, err := readParamArgs(paramPath)
	if err != nil {
		return "", nil, err
	}
	return target, targetArgs, nil
}

func readParamArgs(paramPath string) ([]string, error) {
	contentBytes, err := os.ReadFile(paramPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return parseParamArgs(string(contentBytes))
}

func parseParamArgs(content string) ([]string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, nil
	}

	var args []string
	if err := json.Unmarshal([]byte(content), &args); err != nil {
		return nil, err
	}

	return args, nil
}

func UnicodeString(s string) *uint16 {
	encoded := utf16.Encode([]rune(s + "\x00"))
	return &encoded[0]
}

func showError(message string) {
	modaless, _ := syscall.UTF16PtrFromString(messageBoxTitle)
	msg, _ := syscall.UTF16PtrFromString(message)
	procMessageBoxW.Call(0,
		uintptr(unsafe.Pointer(msg)),
		uintptr(unsafe.Pointer(modaless)),
		messageBoxFlags)
}
