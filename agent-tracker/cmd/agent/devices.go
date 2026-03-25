package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const defaultManagedDeviceID = "web-server"

var managedDevicePattern = regexp.MustCompile(`[^a-z0-9._-]+`)

func normalizeManagedDeviceID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "-")
	value = managedDevicePattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-._")
	return value
}

func normalizeManagedDevices(values []string) []string {
	seen := map[string]bool{defaultManagedDeviceID: true}
	devices := []string{defaultManagedDeviceID}
	for _, value := range values {
		deviceID := normalizeManagedDeviceID(value)
		if deviceID == "" || seen[deviceID] {
			continue
		}
		seen[deviceID] = true
		devices = append(devices, deviceID)
	}
	return devices
}

func loadManagedDevices() []string {
	return normalizeManagedDevices(loadAppConfig().Devices)
}

func saveManagedDevices(devices []string) error {
	return updateAppConfig(func(cfg *appConfig) {
		cfg.Devices = normalizeManagedDevices(devices)
	})
}

func addManagedDevice(deviceID string) error {
	deviceID = normalizeManagedDeviceID(deviceID)
	if deviceID == "" {
		return fmt.Errorf("device id is required")
	}
	devices := loadManagedDevices()
	for _, existing := range devices {
		if existing == deviceID {
			return fmt.Errorf("device %q already exists", deviceID)
		}
	}
	devices = append(devices, deviceID)
	return saveManagedDevices(devices)
}

func removeManagedDevice(deviceID string) error {
	deviceID = normalizeManagedDeviceID(deviceID)
	if deviceID == "" {
		return fmt.Errorf("device id is required")
	}
	if deviceID == defaultManagedDeviceID {
		return fmt.Errorf("%s cannot be removed", defaultManagedDeviceID)
	}
	devices := loadManagedDevices()
	filtered := make([]string, 0, len(devices))
	removed := false
	for _, existing := range devices {
		if existing == deviceID {
			removed = true
			continue
		}
		filtered = append(filtered, existing)
	}
	if !removed {
		return fmt.Errorf("device %q not found", deviceID)
	}
	return saveManagedDevices(filtered)
}

func updateAppConfig(update func(*appConfig)) error {
	cfg := loadAppConfig()
	update(&cfg)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
