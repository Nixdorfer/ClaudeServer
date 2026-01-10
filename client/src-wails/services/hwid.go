package services

import (
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
	"sync"
)

var (
	hwid     string
	hwidOnce sync.Once
)

func GetHwid() string {
	hwidOnce.Do(func() {
		hwid = generateHwid()
	})
	return hwid
}

func generateHwid() string {
	var components []string
	if cpuId := getCpuId(); cpuId != "" {
		components = append(components, cpuId)
	}
	if diskSerial := getDiskSerial(); diskSerial != "" {
		components = append(components, diskSerial)
	}
	if mac := getMacAddress(); mac != "" {
		components = append(components, mac)
	}
	if mbSerial := getMotherboardSerial(); mbSerial != "" {
		components = append(components, mbSerial)
	}
	if len(components) == 0 {
		components = append(components, generateFallbackId())
	}
	combined := strings.Join(components, "|")
	return hashString(combined)
}

func getCpuId() string {
	switch runtime.GOOS {
	case "windows":
		return runWmicCommand("cpu", "get", "ProcessorId")
	case "linux":
		content, err := os.ReadFile("/proc/cpuinfo")
		if err != nil {
			return ""
		}
		for _, line := range strings.Split(string(content), "\n") {
			if strings.HasPrefix(line, "Serial") || strings.Contains(line, "model name") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					return strings.TrimSpace(parts[1])
				}
			}
		}
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return ""
}

func getDiskSerial() string {
	switch runtime.GOOS {
	case "windows":
		return runWmicCommand("diskdrive", "get", "SerialNumber")
	case "linux":
		entries, err := os.ReadDir("/dev/disk/by-id")
		if err != nil {
			return ""
		}
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasPrefix(name, "ata-") || strings.HasPrefix(name, "nvme-") {
				return name
			}
		}
	case "darwin":
		out, err := exec.Command("system_profiler", "SPSerialATADataType").Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, "Serial Number") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						return strings.TrimSpace(parts[1])
					}
				}
			}
		}
	}
	return ""
}

func getMacAddress() string {
	switch runtime.GOOS {
	case "windows":
		out, err := exec.Command("getmac", "/fo", "csv", "/nh").Output()
		if err != nil {
			return ""
		}
		for _, line := range strings.Split(string(out), "\n") {
			parts := strings.Split(line, ",")
			if len(parts) > 0 {
				mac := strings.Trim(strings.TrimSpace(parts[0]), "\"")
				if mac != "" && strings.Contains(mac, "-") {
					return mac
				}
			}
		}
	case "linux":
		entries, err := os.ReadDir("/sys/class/net")
		if err != nil {
			return ""
		}
		for _, entry := range entries {
			name := entry.Name()
			if name == "lo" {
				continue
			}
			addrPath := fmt.Sprintf("/sys/class/net/%s/address", name)
			content, err := os.ReadFile(addrPath)
			if err == nil {
				mac := strings.TrimSpace(string(content))
				if mac != "" && mac != "00:00:00:00:00:00" {
					return mac
				}
			}
		}
	case "darwin":
		out, err := exec.Command("ifconfig", "en0").Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, "ether") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						return fields[1]
					}
				}
			}
		}
	}
	return ""
}

func getMotherboardSerial() string {
	switch runtime.GOOS {
	case "windows":
		result := runWmicCommand("baseboard", "get", "SerialNumber")
		if result != "" && result != "To be filled by O.E.M." {
			return result
		}
	case "linux":
		content, err := os.ReadFile("/sys/class/dmi/id/board_serial")
		if err == nil {
			return strings.TrimSpace(string(content))
		}
	case "darwin":
		out, err := exec.Command("ioreg", "-l").Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, "IOPlatformSerialNumber") {
					start := strings.Index(line, "\"")
					if start != -1 {
						rest := line[start+1:]
						end := strings.Index(rest, "\"")
						if end != -1 {
							return rest[:end]
						}
					}
				}
			}
		}
	}
	return ""
}

func runWmicCommand(args ...string) string {
	cmd := exec.Command("wmic", args...)
	cmd.SysProcAttr = getHideWindowAttr()
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(string(out), "\n")
	for i, line := range lines {
		if i == 0 {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func generateFallbackId() string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	username := "unknown"
	if u, err := user.Current(); err == nil {
		username = u.Username
	}
	return fmt.Sprintf("%s@%s", username, hostname)
}

func hashString(input string) string {
	h1 := fnv.New64a()
	h1.Write([]byte(input))
	hash1 := h1.Sum64()
	reversed := reverseString(input)
	h2 := fnv.New64a()
	h2.Write([]byte(reversed))
	hash2 := h2.Sum64()
	return fmt.Sprintf("%016x%016x", hash1, hash2)
}

func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}
