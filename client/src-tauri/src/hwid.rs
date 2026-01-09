use once_cell::sync::Lazy;
use std::process::Command;

static HWID: Lazy<String> = Lazy::new(|| generate_hwid());

pub fn get_hwid() -> &'static str {
    &HWID
}

fn generate_hwid() -> String {
    let mut components = Vec::new();

    // Get CPU ID
    if let Some(cpu_id) = get_cpu_id() {
        components.push(cpu_id);
    }

    // Get disk serial
    if let Some(disk_serial) = get_disk_serial() {
        components.push(disk_serial);
    }

    // Get MAC address
    if let Some(mac) = get_mac_address() {
        components.push(mac);
    }

    // Get motherboard serial
    if let Some(mb_serial) = get_motherboard_serial() {
        components.push(mb_serial);
    }

    // If no components found, generate a fallback ID
    if components.is_empty() {
        components.push(generate_fallback_id());
    }

    // Combine all components and hash them
    let combined = components.join("|");
    hash_string(&combined)
}

fn get_cpu_id() -> Option<String> {
    #[cfg(target_os = "windows")]
    {
        let output = Command::new("wmic")
            .args(["cpu", "get", "ProcessorId"])
            .output()
            .ok()?;

        let text = String::from_utf8_lossy(&output.stdout);
        for line in text.lines().skip(1) {
            let trimmed = line.trim();
            if !trimmed.is_empty() {
                return Some(trimmed.to_string());
            }
        }
    }

    #[cfg(target_os = "linux")]
    {
        if let Ok(content) = std::fs::read_to_string("/proc/cpuinfo") {
            for line in content.lines() {
                if line.starts_with("Serial") || line.contains("model name") {
                    if let Some(value) = line.split(':').nth(1) {
                        return Some(value.trim().to_string());
                    }
                }
            }
        }
    }

    #[cfg(target_os = "macos")]
    {
        let output = Command::new("sysctl")
            .args(["-n", "machdep.cpu.brand_string"])
            .output()
            .ok()?;

        let text = String::from_utf8_lossy(&output.stdout);
        let trimmed = text.trim();
        if !trimmed.is_empty() {
            return Some(trimmed.to_string());
        }
    }

    #[cfg(target_os = "android")]
    {
        // Android: try to get Build.SERIAL or ANDROID_ID
        let output = Command::new("getprop")
            .args(["ro.serialno"])
            .output()
            .ok()?;

        let text = String::from_utf8_lossy(&output.stdout);
        let trimmed = text.trim();
        if !trimmed.is_empty() && trimmed != "unknown" {
            return Some(trimmed.to_string());
        }
    }

    None
}

fn get_disk_serial() -> Option<String> {
    #[cfg(target_os = "windows")]
    {
        let output = Command::new("wmic")
            .args(["diskdrive", "get", "SerialNumber"])
            .output()
            .ok()?;

        let text = String::from_utf8_lossy(&output.stdout);
        for line in text.lines().skip(1) {
            let trimmed = line.trim();
            if !trimmed.is_empty() {
                return Some(trimmed.to_string());
            }
        }
    }

    #[cfg(target_os = "linux")]
    {
        // Try to get disk ID from /dev/disk/by-id
        if let Ok(entries) = std::fs::read_dir("/dev/disk/by-id") {
            for entry in entries.flatten() {
                let name = entry.file_name().to_string_lossy().to_string();
                if name.starts_with("ata-") || name.starts_with("nvme-") {
                    return Some(name);
                }
            }
        }
    }

    #[cfg(target_os = "macos")]
    {
        let output = Command::new("system_profiler")
            .args(["SPSerialATADataType"])
            .output()
            .ok()?;

        let text = String::from_utf8_lossy(&output.stdout);
        for line in text.lines() {
            if line.contains("Serial Number") {
                if let Some(value) = line.split(':').nth(1) {
                    return Some(value.trim().to_string());
                }
            }
        }
    }

    None
}

fn get_mac_address() -> Option<String> {
    #[cfg(target_os = "windows")]
    {
        let output = Command::new("getmac")
            .args(["/fo", "csv", "/nh"])
            .output()
            .ok()?;

        let text = String::from_utf8_lossy(&output.stdout);
        for line in text.lines() {
            let parts: Vec<&str> = line.split(',').collect();
            if let Some(mac) = parts.first() {
                let mac = mac.trim().trim_matches('"');
                if !mac.is_empty() && mac.contains('-') {
                    return Some(mac.to_string());
                }
            }
        }
    }

    #[cfg(any(target_os = "linux", target_os = "android"))]
    {
        if let Ok(entries) = std::fs::read_dir("/sys/class/net") {
            for entry in entries.flatten() {
                let name = entry.file_name().to_string_lossy().to_string();
                if name == "lo" {
                    continue;
                }
                let addr_path = entry.path().join("address");
                if let Ok(mac) = std::fs::read_to_string(addr_path) {
                    let mac = mac.trim();
                    if !mac.is_empty() && mac != "00:00:00:00:00:00" {
                        return Some(mac.to_string());
                    }
                }
            }
        }
    }

    #[cfg(target_os = "macos")]
    {
        let output = Command::new("ifconfig")
            .args(["en0"])
            .output()
            .ok()?;

        let text = String::from_utf8_lossy(&output.stdout);
        for line in text.lines() {
            if line.contains("ether") {
                if let Some(mac) = line.split_whitespace().nth(1) {
                    return Some(mac.to_string());
                }
            }
        }
    }

    None
}

fn get_motherboard_serial() -> Option<String> {
    #[cfg(target_os = "windows")]
    {
        let output = Command::new("wmic")
            .args(["baseboard", "get", "SerialNumber"])
            .output()
            .ok()?;

        let text = String::from_utf8_lossy(&output.stdout);
        for line in text.lines().skip(1) {
            let trimmed = line.trim();
            if !trimmed.is_empty() && trimmed != "To be filled by O.E.M." {
                return Some(trimmed.to_string());
            }
        }
    }

    #[cfg(target_os = "linux")]
    {
        if let Ok(serial) = std::fs::read_to_string("/sys/class/dmi/id/board_serial") {
            let trimmed = serial.trim();
            if !trimmed.is_empty() {
                return Some(trimmed.to_string());
            }
        }
    }

    #[cfg(target_os = "macos")]
    {
        let output = Command::new("ioreg")
            .args(["-l"])
            .output()
            .ok()?;

        let text = String::from_utf8_lossy(&output.stdout);
        for line in text.lines() {
            if line.contains("IOPlatformSerialNumber") {
                if let Some(start) = line.find('"') {
                    let rest = &line[start + 1..];
                    if let Some(end) = rest.find('"') {
                        return Some(rest[..end].to_string());
                    }
                }
            }
        }
    }

    None
}

fn generate_fallback_id() -> String {
    // Use hostname + username as fallback
    let hostname = hostname::get()
        .map(|h| h.to_string_lossy().to_string())
        .unwrap_or_else(|_| "unknown".to_string());

    let username = whoami::username();

    format!("{}@{}", username, hostname)
}

fn hash_string(input: &str) -> String {
    use std::collections::hash_map::DefaultHasher;
    use std::hash::{Hash, Hasher};

    let mut hasher = DefaultHasher::new();
    input.hash(&mut hasher);
    let hash1 = hasher.finish();

    // Do a second pass with different seed
    let reversed: String = input.chars().rev().collect();
    let mut hasher2 = DefaultHasher::new();
    reversed.hash(&mut hasher2);
    let hash2 = hasher2.finish();

    format!("{:016x}{:016x}", hash1, hash2)
}
