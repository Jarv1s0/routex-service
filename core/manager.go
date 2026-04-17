package core

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

const (
	startupGracePeriod = 500 * time.Millisecond
	stopWaitTimeout    = 5 * time.Second
	restartRetryDelay  = 2 * time.Second
	restartMaxRetries  = 3
)

type StartConfig struct {
	BinaryPath string            `json:"binary_path"`
	WorkDir    string            `json:"work_dir,omitempty"`
	LogPath    string            `json:"log_path,omitempty"`
	Args       []string          `json:"args,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
}

func (cfg *StartConfig) IsEmpty() bool {
	if cfg == nil {
		return true
	}

	return strings.TrimSpace(cfg.BinaryPath) == "" &&
		strings.TrimSpace(cfg.WorkDir) == "" &&
		strings.TrimSpace(cfg.LogPath) == "" &&
		len(cfg.Args) == 0 &&
		len(cfg.Env) == 0
}

func (cfg *StartConfig) Clone() *StartConfig {
	if cfg == nil {
		return nil
	}

	cloned := &StartConfig{
		BinaryPath: cfg.BinaryPath,
		WorkDir:    cfg.WorkDir,
		LogPath:    cfg.LogPath,
		Args:       append([]string(nil), cfg.Args...),
	}

	if len(cfg.Env) > 0 {
		cloned.Env = make(map[string]string, len(cfg.Env))
		for key, value := range cfg.Env {
			cloned.Env[key] = value
		}
	}

	return cloned
}

func (cfg *StartConfig) Validate() error {
	if cfg == nil || strings.TrimSpace(cfg.BinaryPath) == "" {
		return fmt.Errorf("缺少内核二进制路径")
	}

	cfg.BinaryPath = strings.TrimSpace(cfg.BinaryPath)
	cfg.WorkDir = strings.TrimSpace(cfg.WorkDir)
	cfg.LogPath = strings.TrimSpace(cfg.LogPath)

	if _, err := os.Stat(cfg.BinaryPath); err != nil {
		return fmt.Errorf("内核二进制不存在: %w", err)
	}

	if cfg.WorkDir == "" {
		cfg.WorkDir = filepath.Dir(cfg.BinaryPath)
	}

	if err := os.MkdirAll(cfg.WorkDir, 0o755); err != nil {
		return fmt.Errorf("创建内核工作目录失败: %w", err)
	}

	if cfg.LogPath != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.LogPath), 0o755); err != nil {
			return fmt.Errorf("创建日志目录失败: %w", err)
		}
	}

	return nil
}

type CoreManager struct {
	cmd        *exec.Cmd
	config     *StartConfig
	isRunning  atomic.Bool
	monitoring atomic.Bool
	startTime  time.Time
	pid        atomic.Int32
	mutex      sync.Mutex
	stopChan   chan struct{}
}

type ProcessInfo struct {
	PID          int32     `json:"pid"`
	Memory       uint64    `json:"memory"`
	MemoryFormat string    `json:"memory_format"`
	StartTime    time.Time `json:"start_time"`
	Uptime       string    `json:"uptime"`
	BinaryPath   string    `json:"binary_path,omitempty"`
	WorkDir      string    `json:"work_dir,omitempty"`
	LogPath      string    `json:"log_path,omitempty"`
}

func NewCoreManager() *CoreManager {
	return &CoreManager{
		stopChan: make(chan struct{}),
	}
}

func (cm *CoreManager) StartCore(config *StartConfig) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	if cm.isRunning.Load() {
		return fmt.Errorf("核心进程已在运行中")
	}

	nextConfig := cm.resolveConfig(config)
	if err := nextConfig.Validate(); err != nil {
		return err
	}

	cmd, err := cm.buildCommand(nextConfig)
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动核心进程失败: %w", err)
	}

	cm.cmd = cmd
	cm.config = nextConfig.Clone()
	cm.stopChan = make(chan struct{})
	cm.startTime = time.Now()
	cm.pid.Store(int32(cmd.Process.Pid))
	cm.isRunning.Store(true)
	cm.monitoring.Store(true)

	if err := cm.waitForStartup(cmd.Process.Pid); err != nil {
		cm.monitoring.Store(false)
		_ = cm.stopProcess()
		cm.cleanupLocked()
		return err
	}

	go cm.monitorProcess(cmd, nextConfig.Clone())
	return nil
}

func (cm *CoreManager) StopCore() error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	if !cm.isRunning.Load() {
		return nil
	}

	cm.monitoring.Store(false)
	close(cm.stopChan)

	if err := cm.stopProcess(); err != nil {
		return err
	}

	cm.cleanupLocked()
	return nil
}

func (cm *CoreManager) RestartCore(config *StartConfig) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	nextConfig := cm.resolveConfig(config)
	if err := nextConfig.Validate(); err != nil {
		return err
	}

	if cm.isRunning.Load() {
		cm.monitoring.Store(false)
		close(cm.stopChan)
		if err := cm.stopProcess(); err != nil {
			return err
		}
		cm.cleanupLocked()
		time.Sleep(200 * time.Millisecond)
	}

	cmd, err := cm.buildCommand(nextConfig)
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动核心进程失败: %w", err)
	}

	cm.cmd = cmd
	cm.config = nextConfig.Clone()
	cm.stopChan = make(chan struct{})
	cm.startTime = time.Now()
	cm.pid.Store(int32(cmd.Process.Pid))
	cm.isRunning.Store(true)
	cm.monitoring.Store(true)

	if err := cm.waitForStartup(cmd.Process.Pid); err != nil {
		cm.monitoring.Store(false)
		_ = cm.stopProcess()
		cm.cleanupLocked()
		return err
	}

	go cm.monitorProcess(cmd, nextConfig.Clone())
	return nil
}

func (cm *CoreManager) GetProcessInfo() (*ProcessInfo, error) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	if !cm.isRunning.Load() || cm.pid.Load() <= 0 || !isProcessRunning(cm.pid.Load()) {
		cm.cleanupLocked()
		return nil, fmt.Errorf("进程未运行")
	}

	proc, err := process.NewProcess(cm.pid.Load())
	if err != nil {
		cm.cleanupLocked()
		return nil, fmt.Errorf("获取进程信息失败: %w", err)
	}

	info := &ProcessInfo{
		PID:       cm.pid.Load(),
		StartTime: cm.startTime,
		Uptime:    formatUptime(time.Since(cm.startTime)),
	}

	if cm.config != nil {
		info.BinaryPath = cm.config.BinaryPath
		info.WorkDir = cm.config.WorkDir
		info.LogPath = cm.config.LogPath
	}

	if memInfo, err := proc.MemoryInfo(); err == nil {
		info.Memory = memInfo.RSS
		info.MemoryFormat = formatMemory(memInfo.RSS)
	}

	return info, nil
}

func (cm *CoreManager) resolveConfig(config *StartConfig) *StartConfig {
	if config != nil && !config.IsEmpty() {
		return config.Clone()
	}
	if cm.config != nil {
		return cm.config.Clone()
	}
	return &StartConfig{}
}

func (cm *CoreManager) buildCommand(config *StartConfig) (*exec.Cmd, error) {
	cmd := exec.Command(config.BinaryPath, config.Args...)
	cmd.Dir = config.WorkDir

	env := append([]string(nil), os.Environ()...)
	for key, value := range config.Env {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}
	cmd.Env = env

	if config.LogPath != "" {
		stdout, err := os.OpenFile(config.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, fmt.Errorf("打开日志文件失败: %w", err)
		}

		stderr, err := os.OpenFile(config.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			_ = stdout.Close()
			return nil, fmt.Errorf("打开日志文件失败: %w", err)
		}

		cmd.Stdout = stdout
		cmd.Stderr = stderr
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	return cmd, nil
}

func (cm *CoreManager) waitForStartup(pid int) error {
	deadline := time.Now().Add(startupGracePeriod)
	for time.Now().Before(deadline) {
		if !isProcessRunning(int32(pid)) {
			return fmt.Errorf("核心进程启动后立即退出")
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !isProcessRunning(int32(pid)) {
		return fmt.Errorf("核心进程启动失败")
	}

	return nil
}

func (cm *CoreManager) monitorProcess(cmd *exec.Cmd, config *StartConfig) {
	err := cmd.Wait()
	if !cm.monitoring.Load() || cmd.Process == nil || cm.pid.Load() != int32(cmd.Process.Pid) {
		return
	}

	if err != nil {
		log.Printf("核心进程异常退出: %v", err)
	} else {
		log.Printf("核心进程已退出 (PID: %d)", cmd.Process.Pid)
	}

	cm.mutex.Lock()
	cm.cleanupLocked()
	cm.mutex.Unlock()

	go cm.restartWithBackoff(config)
}

func (cm *CoreManager) restartWithBackoff(config *StartConfig) {
	if config == nil {
		return
	}

	for attempt := range restartMaxRetries {
		if !cm.monitoring.Load() {
			return
		}

		time.Sleep(restartRetryDelay)
		if err := cm.StartCore(config); err != nil {
			log.Printf("重启核心进程失败 (尝试 %d/%d): %v", attempt+1, restartMaxRetries, err)
			continue
		}

		log.Println("核心进程已成功重启")
		return
	}

	log.Println("达到最大重试次数，重启失败")
}

func (cm *CoreManager) stopProcess() error {
	pid := cm.pid.Load()
	if pid <= 0 {
		return nil
	}

	if cm.cmd != nil && cm.cmd.Process != nil {
		if err := cm.cmd.Process.Kill(); err == nil {
			return waitForProcessExit(pid)
		}
	}

	if runtime.GOOS == "windows" {
		return stopProcessWindows(pid)
	}

	return stopProcessUnix(pid)
}

func (cm *CoreManager) cleanupLocked() {
	cm.cmd = nil
	cm.config = nil
	cm.startTime = time.Time{}
	cm.pid.Store(0)
	cm.isRunning.Store(false)
}

func stopProcessWindows(pid int32) error {
	cmd := exec.Command("taskkill", "/PID", strconv.Itoa(int(pid)), "/T", "/F")
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := convertGBKToUTF8(output)
		if strings.Contains(outputStr, "没有找到进程") ||
			strings.Contains(strings.ToLower(outputStr), "not found") {
			return nil
		}
		return fmt.Errorf("终止进程失败: %v, output: %s", err, outputStr)
	}

	return waitForProcessExit(pid)
}

func stopProcessUnix(pid int32) error {
	proc, err := os.FindProcess(int(pid))
	if err != nil {
		return nil
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil && !strings.Contains(strings.ToLower(err.Error()), "finished") {
		return fmt.Errorf("终止进程失败: %w", err)
	}

	deadline := time.Now().Add(stopWaitTimeout)
	for time.Now().Before(deadline) {
		if !isProcessRunning(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	if err := proc.Signal(syscall.SIGKILL); err != nil && !strings.Contains(strings.ToLower(err.Error()), "finished") {
		return fmt.Errorf("强制终止进程失败: %w", err)
	}

	return waitForProcessExit(pid)
}

func isProcessRunning(pid int32) bool {
	if pid <= 0 {
		return false
	}

	proc, err := process.NewProcess(pid)
	if err != nil {
		return false
	}

	running, err := proc.IsRunning()
	if err != nil {
		return false
	}

	return running
}

func waitForProcessExit(pid int32) error {
	deadline := time.Now().Add(stopWaitTimeout)
	for time.Now().Before(deadline) {
		if !isProcessRunning(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

func formatMemory(bytes uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	parts := make([]string, 0, 4)
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 || len(parts) > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 || len(parts) > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	parts = append(parts, fmt.Sprintf("%ds", seconds))

	return strings.Join(parts, " ")
}

func convertGBKToUTF8(b []byte) string {
	reader := transform.NewReader(strings.NewReader(string(b)), simplifiedchinese.GBK.NewDecoder())
	output, err := io.ReadAll(reader)
	if err != nil {
		return string(b)
	}
	return string(output)
}
