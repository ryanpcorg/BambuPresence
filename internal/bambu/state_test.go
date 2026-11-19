package bambu

import "testing"

func TestFanRawToPercent(t *testing.T) {
	cases := []struct {
		raw  int
		want int
	}{
		{0, 0},
		{3, 20},
		{7, 50},
		{12, 80},
		{15, 100},
	}
	for _, tc := range cases {
		if got := FanRawToPercent(tc.raw); got != tc.want {
			t.Fatalf("FanRawToPercent(%d) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}

func TestParseStateAMSAndTelemetry(t *testing.T) {
	raw := map[string]any{
		"gcode_state":          "RUNNING",
		"st_id":                0,
		"mc_percent":           42,
		"mc_remaining_time":    75,
		"layer_num":            120,
		"total_layer_num":      300,
		"subtask_name":         "benchy.3mf",
		"nozzle_temper":        220.4,
		"nozzle_target_temper": 220.0,
		"bed_temper":           60.1,
		"bed_target_temper":    60.0,
		"chamber_temper":       34.5,
		"cooling_fan_speed":    "15",
		"big_fan1_speed":       "7",
		"big_fan2_speed":       "3",
		"spd_lvl":              3,
		"spd_mag":              124,
		"ams": map[string]any{
			"tray_now": "1",
			"ams": []any{
				map[string]any{
					"id":       "0",
					"temp":     "26.5",
					"humidity": "2",
					"tray": []any{
						map[string]any{
							"id":              "0",
							"tray_type":       "PLA",
							"tray_sub_brands": "PLA Basic",
							"tray_color":      "FF0000FF",
							"remain":          80,
						},
						map[string]any{
							"id":              "1",
							"tray_type":       "PETG",
							"tray_sub_brands": "PETG HF",
							"tray_color":      "00AE42FF",
							"remain":          55,
						},
					},
				},
			},
		},
	}

	st := ParseState(raw, "01P00A123456", nil, true)
	if st.Model.Name != "Bambu Lab P1S" {
		t.Fatalf("model = %q, want Bambu Lab P1S", st.Model.Name)
	}
	if st.CoolingFanPct != 100 || st.AuxFanPct != 50 || st.ChamberFanPct != 20 {
		t.Fatalf("fans = %d/%d/%d, want 100/50/20", st.CoolingFanPct, st.AuxFanPct, st.ChamberFanPct)
	}
	if st.SpeedName != "Sport" {
		t.Fatalf("speed = %q, want Sport", st.SpeedName)
	}
	if got := st.ActiveFilamentLabel(); got != "PETG HF (Green, 55%, AMS 1-2)" {
		t.Fatalf("active filament = %q, want PETG HF (Green, 55%%, AMS 1-2)", got)
	}
}
