package bambu

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	hmsFileName = "hms_en.json.gz"
	hmsURL      = "https://raw.githubusercontent.com/greghesp/ha-bambulab/main/custom_components/bambu_lab/pybambu/hms_error_text/hms_en.json.gz"
)

type HMSError struct {
	Code    string
	Message string
}

type HMSDictionary struct {
	entries map[string]string
}

func LoadOrDownloadHMS(onLog func(string)) *HMSDictionary {
	dict := &HMSDictionary{
		entries: make(map[string]string),
	}

	if _, err := os.Stat(hmsFileName); os.IsNotExist(err) {
		if onLog != nil {
			onLog("Downloading HMS error dictionary...")
		}
		if err := downloadFile(hmsURL, hmsFileName); err != nil {
			if onLog != nil {
				onLog(fmt.Sprintf("Failed to download HMS dictionary: %v", err))
			}
			return dict
		}
		if onLog != nil {
			onLog("HMS error dictionary downloaded.")
		}
	}

	f, err := os.Open(hmsFileName)
	if err != nil {
		if onLog != nil {
			onLog(fmt.Sprintf("Error opening HMS dictionary: %v", err))
		}
		return dict
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		if onLog != nil {
			onLog(fmt.Sprintf("Error reading HMS gzip: %v", err))
		}
		return dict
	}
	defer gz.Close()

	var raw struct {
		DeviceHMS map[string]map[string]any `json:"device_hms"`
	}
	if err := json.NewDecoder(gz).Decode(&raw); err != nil {
		if onLog != nil {
			onLog(fmt.Sprintf("Error decoding HMS JSON: %v", err))
		}
		return dict
	}

	for code, msgMap := range raw.DeviceHMS {
		for msg := range msgMap {
			dict.entries[code] = msg
			break
		}
	}
	return dict
}

func (d *HMSDictionary) Lookup(attr, code uint64) HMSError {
	part1 := (attr >> 16) & 0xFFFF
	part2 := attr & 0xFFFF
	part3 := (code >> 16) & 0xFFFF
	part4 := code & 0xFFFF
	hexCode := fmt.Sprintf("%04X%04X%04X%04X", part1, part2, part3, part4)

	if d != nil {
		if msg, ok := d.entries[hexCode]; ok && msg != "" {
			return HMSError{
				Code:    hexCode,
				Message: msg,
			}
		}
	}
	return HMSError{
		Code:    hexCode,
		Message: fmt.Sprintf("Unknown Error (HMS: %s)", hexCode),
	}
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
