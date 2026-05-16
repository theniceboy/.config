package main

import (
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"time"
)

func defaultBrowserScreenshotPath(featurePath string) (string, error) {
	name := sanitizeFeatureName(filepath.Base(filepath.Dir(featurePath)))
	if name == "" {
		name = "agent"
	}
	return filepath.Join(os.TempDir(), "agent-browser-"+name+"-"+time.Now().Format("20060102-150405.000000000")+".jpg"), nil
}

func imageDimensions(data []byte) (int, int, bool) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, false
	}
	return config.Width, config.Height, true
}
