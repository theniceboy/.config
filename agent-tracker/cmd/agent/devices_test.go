package main

import "testing"

func TestLoadManagedDevicesDefaultsToWebServer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	devices := loadManagedDevices()
	if len(devices) != 1 || devices[0] != defaultManagedDeviceID {
		t.Fatalf("unexpected default devices: %#v", devices)
	}
}

func TestSaveManagedDevicesKeepsWebServerFirst(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := saveManagedDevices([]string{"ios", "web-server", "macos", "IOS"}); err != nil {
		t.Fatalf("save managed devices: %v", err)
	}
	devices := loadManagedDevices()
	if len(devices) != 3 {
		t.Fatalf("unexpected devices length: %#v", devices)
	}
	if devices[0] != defaultManagedDeviceID || devices[1] != "ios" || devices[2] != "macos" {
		t.Fatalf("unexpected normalized devices: %#v", devices)
	}
}

func TestRemoveManagedDeviceRejectsWebServer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := removeManagedDevice(defaultManagedDeviceID); err == nil {
		t.Fatal("expected web-server removal to fail")
	}
}
