package bambu

import (
	"math"
	"strings"
)

type PrinterModel struct {
	Name     string
	ImageURL string
}

const DefaultBrandImageURL = "https://brands.home-assistant.io/bambu_lab/icon.png"

var StageMapping = map[int]string{
	0:  "Printing",
	1:  "Auto bed leveling",
	2:  "Heatbed preheating",
	3:  "Sweeping XY mech mode",
	4:  "Changing filament",
	5:  "M400 pause",
	6:  "Paused due to filament runout",
	7:  "Heating hotend",
	8:  "Calibrating extrusion",
	9:  "Scanning bed surface",
	10: "Inspecting first layer",
	11: "Identifying build plate type",
	12: "Calibrating Micro Lidar",
	13: "Homing toolhead",
	14: "Cleaning nozzle tip",
	15: "Checking extruder temperature",
	16: "Paused by user",
	17: "Pause of front cover falling",
	18: "Calibrating Micro Lidar",
	19: "Calibrating extrusion flow",
	20: "Paused due to nozzle temperature malfunction",
	21: "Paused due to heat bed temperature malfunction",
	22: "Filament unloading",
	23: "Pause of skipped step",
	24: "Filament loading",
	25: "Calibrating motor noise",
	26: "Paused due to AMS lost",
	27: "Paused due to low fan speed of heat break",
	28: "Paused due to chamber temperature control error",
	29: "Cooling chamber",
	30: "Paused by user Gcode",
	31: "Motor noise showoff",
	32: "Paused due to nozzle filament covered detected",
	33: "Paused due to cutter error",
	34: "Paused due to first layer error",
	35: "Paused due to nozzle clog",
}

var SpeedProfileNames = map[int]string{
	1: "Silent",
	2: "Standard",
	3: "Sport",
	4: "Ludicrous",
}

const modelImageBase = "https://raw.githubusercontent.com/greghesp/ha-bambulab-cards/main/src/images/"

func DetectPrinterModel(serial string) PrinterModel {
	s := strings.ToUpper(strings.TrimSpace(serial))
	switch {
	case strings.HasPrefix(s, "00M"), strings.HasPrefix(s, "00W"):
		return PrinterModel{
			Name:     "Bambu Lab X1 Carbon",
			ImageURL: modelImageBase + "X1C.webp",
		}
	case strings.HasPrefix(s, "03W"), strings.HasPrefix(s, "00E"):
		return PrinterModel{
			Name:     "Bambu Lab X1E",
			ImageURL: modelImageBase + "X1E.webp",
		}
	case strings.HasPrefix(s, "01S"):
		return PrinterModel{
			Name:     "Bambu Lab P1P",
			ImageURL: modelImageBase + "P1P.webp",
		}
	case strings.HasPrefix(s, "01P"):
		return PrinterModel{
			Name:     "Bambu Lab P1S",
			ImageURL: modelImageBase + "P1S.webp",
		}
	case strings.HasPrefix(s, "039"):
		return PrinterModel{
			Name:     "Bambu Lab A1",
			ImageURL: modelImageBase + "A1.webp",
		}
	case strings.HasPrefix(s, "030"):
		return PrinterModel{
			Name:     "Bambu Lab A1 mini",
			ImageURL: modelImageBase + "A1Mini.webp",
		}
	case strings.HasPrefix(s, "094"):
		return PrinterModel{
			Name:     "Bambu Lab H2D",
			ImageURL: modelImageBase + "H2D.webp",
		}
	default:
		return PrinterModel{
			Name:     "Bambu Lab Printer",
			ImageURL: DefaultBrandImageURL,
		}
	}
}

func FanRawToPercent(raw int) int {
	if raw <= 0 {
		return 0
	}
	if raw >= 15 {
		return 100
	}
	pct := math.Ceil(((float64(raw)/15.0)*100.0)/10.0) * 10.0
	if pct > 100 {
		return 100
	}
	return int(pct)
}

func SpeedProfileName(level int) string {
	if name, ok := SpeedProfileNames[level]; ok {
		return name
	}
	return "Standard"
}
