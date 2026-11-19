package bambu

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type AMSTray struct {
	ID            int
	Type          string
	SubBrand      string
	ColorHex      string
	Remain        int
	NozzleTempMin int
	NozzleTempMax int
	Active        bool
	Empty         bool
}

type AMSUnit struct {
	ID       int
	Temp     float64
	Humidity int
	Trays    []AMSTray
}

type PrinterState struct {
	Connected       bool
	GcodeState      string
	StageID         int
	StageText       string
	Percent         int
	RemainingMins   int
	Layer           int
	TotalLayers     int
	SubtaskName     string
	NozzleTemp      float64
	NozzleTarget    float64
	BedTemp         float64
	BedTarget       float64
	ChamberTemp     float64
	CoolingFanPct   int
	AuxFanPct       int
	ChamberFanPct   int
	HeatbreakFanPct int
	SpeedLevel      int
	SpeedName       string
	SpeedMag        int
	ChamberLightOn  bool
	WiFiSignal      string
	ActiveTrayIdx   int
	AMSUnits        []AMSUnit
	ExternalSpool   *AMSTray
	Errors          []HMSError
	Model           PrinterModel
	LastUpdated     time.Time
}

func (s PrinterState) ActiveFilamentLabel() string {
	if s.ActiveTrayIdx == 254 && s.ExternalSpool != nil && !s.ExternalSpool.Empty {
		return formatTrayLabel(*s.ExternalSpool, "Ext")
	}
	if s.ActiveTrayIdx >= 0 && s.ActiveTrayIdx < 254 {
		amsIdx := s.ActiveTrayIdx / 4
		slotIdx := s.ActiveTrayIdx % 4
		for _, unit := range s.AMSUnits {
			if unit.ID == amsIdx {
				for _, t := range unit.Trays {
					if t.ID == slotIdx && !t.Empty {
						return formatTrayLabel(t, fmt.Sprintf("AMS %d-%d", amsIdx+1, slotIdx+1))
					}
				}
			}
		}
	}
	return ""
}

func (s PrinterState) IsAlertStage() bool {
	switch s.StageID {
	case 6, 17, 20, 21, 23, 26, 27, 28, 32, 33, 34, 35:
		return true
	default:
		return false
	}
}

func formatTrayLabel(t AMSTray, prefix string) string {
	name := strings.TrimSpace(t.SubBrand)
	if name == "" {
		name = strings.TrimSpace(t.Type)
	}
	if name == "" {
		return prefix
	}
	colName := colorHexToName(t.ColorHex)
	var meta []string
	if colName != "" {
		meta = append(meta, colName)
	}
	if t.Remain > 0 {
		meta = append(meta, fmt.Sprintf("%d%%", t.Remain))
	}
	meta = append(meta, prefix)
	return fmt.Sprintf("%s (%s)", name, strings.Join(meta, ", "))
}

func colorHexToName(hex string) string {
	h := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(h) < 6 {
		return ""
	}
	r64, err1 := strconv.ParseUint(h[0:2], 16, 8)
	g64, err2 := strconv.ParseUint(h[2:4], 16, 8)
	b64, err3 := strconv.ParseUint(h[4:6], 16, 8)
	if err1 != nil || err2 != nil || err3 != nil {
		return ""
	}
	r, g, b := int(r64), int(g64), int(b64)

	palette := []struct {
		name    string
		r, g, b int
	}{
		{"Black", 0, 0, 0},
		{"White", 255, 255, 255},
		{"Gray", 128, 128, 128},
		{"Silver", 192, 192, 192},
		{"Red", 220, 38, 38},
		{"Orange", 249, 115, 22},
		{"Yellow", 234, 179, 8},
		{"Green", 22, 163, 74},
		{"Cyan", 6, 182, 212},
		{"Blue", 37, 99, 235},
		{"Purple", 147, 51, 234},
		{"Pink", 236, 72, 153},
		{"Brown", 120, 53, 15},
	}

	bestName := ""
	bestDist := 1 << 30
	for _, p := range palette {
		dr := r - p.r
		dg := g - p.g
		db := b - p.b
		d := dr*dr + dg*dg + db*db
		if d < bestDist {
			bestDist = d
			bestName = p.name
		}
	}
	return bestName
}

func ParseState(raw map[string]any, serial string, hms *HMSDictionary, connected bool) PrinterState {
	st := PrinterState{
		Connected:     connected,
		GcodeState:    getString(raw, "gcode_state", "UNKNOWN"),
		StageID:       255,
		Percent:       getInt(raw, "mc_percent", 0),
		RemainingMins: getInt(raw, "mc_remaining_time", 0),
		Layer:         getInt(raw, "layer_num", 0),
		TotalLayers:   getInt(raw, "total_layer_num", 0),
		SubtaskName:   getString(raw, "subtask_name", ""),
		NozzleTemp:    getFloat(raw, "nozzle_temper", 0),
		NozzleTarget:  getFloat(raw, "nozzle_target_temper", 0),
		BedTemp:       getFloat(raw, "bed_temper", 0),
		BedTarget:     getFloat(raw, "bed_target_temper", 0),
		ChamberTemp:   getFloat(raw, "chamber_temper", 0),
		SpeedLevel:    getInt(raw, "spd_lvl", 2),
		SpeedMag:      getInt(raw, "spd_mag", 100),
		WiFiSignal:    getString(raw, "wifi_signal", ""),
		ActiveTrayIdx: 255,
		Model:         DetectPrinterModel(serial),
		LastUpdated:   time.Now(),
	}

	if _, ok := raw["st_id"]; ok {
		st.StageID = getInt(raw, "st_id", 255)
	} else if _, ok := raw["stg_cur"]; ok {
		st.StageID = getInt(raw, "stg_cur", 255)
	}
	if label, ok := StageMapping[st.StageID]; ok {
		st.StageText = label
	}

	st.SpeedName = SpeedProfileName(st.SpeedLevel)
	st.CoolingFanPct = FanRawToPercent(getInt(raw, "cooling_fan_speed", 0))
	st.AuxFanPct = FanRawToPercent(getInt(raw, "big_fan1_speed", 0))
	st.ChamberFanPct = FanRawToPercent(getInt(raw, "big_fan2_speed", 0))
	st.HeatbreakFanPct = FanRawToPercent(getInt(raw, "heatbreak_fan_speed", 0))

	if lights, ok := raw["lights_report"].([]any); ok {
		for _, item := range lights {
			if m, ok := item.(map[string]any); ok {
				if getString(m, "node", "") == "chamber_light" {
					st.ChamberLightOn = strings.EqualFold(getString(m, "mode", ""), "on")
				}
			}
		}
	}

	if amsObj, ok := raw["ams"].(map[string]any); ok {
		st.ActiveTrayIdx = getInt(amsObj, "tray_now", 255)
		if amsList, ok := amsObj["ams"].([]any); ok {
			for _, u := range amsList {
				uMap, ok := u.(map[string]any)
				if !ok {
					continue
				}
				unit := AMSUnit{
					ID:       getInt(uMap, "id", 0),
					Temp:     getFloat(uMap, "temp", 0),
					Humidity: getInt(uMap, "humidity", 0),
				}
				if trays, ok := uMap["tray"].([]any); ok {
					for _, tr := range trays {
						trMap, ok := tr.(map[string]any)
						if !ok {
							continue
						}
						tray := parseTray(trMap)
						globalIdx := unit.ID*4 + tray.ID
						tray.Active = (st.ActiveTrayIdx == globalIdx)
						unit.Trays = append(unit.Trays, tray)
					}
				}
				st.AMSUnits = append(st.AMSUnits, unit)
			}
		}
	}

	if vtObj, ok := raw["vt_tray"].(map[string]any); ok {
		tray := parseTray(vtObj)
		tray.Active = (st.ActiveTrayIdx == 254)
		st.ExternalSpool = &tray
	}

	if hmsList, ok := raw["hms"].([]any); ok {
		for _, item := range hmsList {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			attr := uint64(getInt(m, "attr", 0))
			code := uint64(getInt(m, "code", 0))
			if attr > 0 && code > 0 {
				st.Errors = append(st.Errors, hms.Lookup(attr, code))
			}
		}
	}

	return st
}

func parseTray(m map[string]any) AMSTray {
	trayType := strings.TrimSpace(getString(m, "tray_type", ""))
	subBrand := strings.TrimSpace(getString(m, "tray_sub_brands", ""))
	colorHex := strings.TrimSpace(getString(m, "tray_color", ""))
	remain := getInt(m, "remain", -1)
	return AMSTray{
		ID:            getInt(m, "id", 0),
		Type:          trayType,
		SubBrand:      subBrand,
		ColorHex:      colorHex,
		Remain:        remain,
		NozzleTempMin: getInt(m, "nozzle_temp_min", 0),
		NozzleTempMax: getInt(m, "nozzle_temp_max", 0),
		Empty:         trayType == "" && colorHex == "",
	}
}

func getString(m map[string]any, key, def string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func getInt(m map[string]any, key string, def int) int {
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return def
		}
		if i, err := strconv.Atoi(s); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int(f)
		}
	}
	return def
}

func getFloat(m map[string]any, key string, def float64) float64 {
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case string:
		s := strings.TrimSpace(val)
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	}
	return def
}
