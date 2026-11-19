package config

import (
	"encoding/json"
	"os"
)

const DefaultConfigPath = "config.json"

type Config struct {
	IP                  string `json:"ip"`
	AccessCode          string `json:"access_code"`
	Serial              string `json:"serial"`
	ClientID            string `json:"client_id"`
	ShowFilename        bool   `json:"show_filename"`
	ShowLayers          bool   `json:"show_layers"`
	ShowTime            bool   `json:"show_time"`
	ShowTemps           bool   `json:"show_temps"`
	ShowErrors          bool   `json:"show_errors"`
	ShowSpecificPrinter bool   `json:"show_specific_printer"`
	ShowFilament        bool   `json:"show_filament"`
	ShowSpeed           bool   `json:"show_speed"`
	UseETACountdown     bool   `json:"use_eta_countdown"`
	HideWhenIdle        bool   `json:"hide_when_idle"`
	CustomDetailsFmt    string `json:"custom_details_fmt"`
	CustomStateFmt      string `json:"custom_state_fmt"`
	AutoConnect         bool   `json:"auto_connect"`
	NotifyFinish        bool   `json:"notify_finish"`
	NotifyError         bool   `json:"notify_error"`
	NotifyPause         bool   `json:"notify_pause"`
	CompletionMinutes   int    `json:"completion_minutes"`
}

func Default() Config {
	return Config{
		ShowFilename:        true,
		ShowLayers:          true,
		ShowTime:            true,
		ShowTemps:           true,
		ShowErrors:          true,
		ShowSpecificPrinter: true,
		ShowFilament:        true,
		ShowSpeed:           false,
		UseETACountdown:     true,
		HideWhenIdle:        false,
		AutoConnect:         false,
		NotifyFinish:        true,
		NotifyError:         true,
		NotifyPause:         true,
		CompletionMinutes:   5,
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if cfg.CompletionMinutes <= 0 {
		cfg.CompletionMinutes = 5
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
