package discord

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"bambupresence/internal/bambu"
	"bambupresence/internal/config"
)

const (
	opHandshake = 0
	opFrame     = 1
	opClose     = 2
)

type PresencePreview struct {
	Connected  bool
	Hidden     bool
	Details    string
	State      string
	LargeImage string
	LargeText  string
	SmallImage string
	SmallText  string
	TimerLabel string
}

type Client struct {
	mu             sync.Mutex
	clientID       string
	cfg            config.Config
	onLog          func(string)
	conn           net.Conn
	connected      bool
	nonce          int64
	printStartTime time.Time
	completionTime time.Time
	lastSentKey    string
	lastSentAt     time.Time
	lastPreview    PresencePreview
}

func NewClient(clientID string, cfg config.Config, onLog func(string)) *Client {
	return &Client{
		clientID: strings.TrimSpace(clientID),
		cfg:      cfg,
		onLog:    onLog,
	}
}

func (c *Client) UpdateConfig(cfg config.Config) {
	c.mu.Lock()
	c.cfg = cfg
	c.lastSentKey = ""
	c.mu.Unlock()
}

func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.clientID == "" {
		c.log("No Discord Client ID provided. Skipping Discord RPC.")
		return nil
	}

	c.closeLocked()
	c.log("Connecting to Discord RPC...")

	conn, err := dialIPC()
	if err != nil {
		c.log(fmt.Sprintf("Discord RPC connection error: %v", err))
		return err
	}

	handshake := map[string]any{
		"v":         1,
		"client_id": c.clientID,
	}
	if err := writePacket(conn, opHandshake, handshake); err != nil {
		conn.Close()
		c.log(fmt.Sprintf("Discord RPC handshake write error: %v", err))
		return err
	}

	if _, _, err := readPacket(conn); err != nil {
		conn.Close()
		c.log(fmt.Sprintf("Discord RPC handshake read error: %v", err))
		return err
	}

	c.conn = conn
	c.connected = true
	c.lastPreview.Connected = true
	c.log("Connected to Discord RPC.")
	return nil
}

func (c *Client) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.connected {
		c.log("Disconnecting from Discord RPC...")
	}
	c.closeLocked()
}

func (c *Client) closeLocked() {
	if c.conn != nil {
		_ = writePacket(c.conn, opClose, map[string]any{})
		_ = c.conn.Close()
		c.conn = nil
	}
	c.connected = false
	c.lastPreview.Connected = false
	c.lastSentKey = ""
}

func (c *Client) Preview() PresencePreview {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastPreview
}

func (c *Client) UpdatePresence(st bambu.PrinterState) PresencePreview {
	c.mu.Lock()
	defer c.mu.Unlock()

	preview, activity, hide := c.buildActivityLocked(st)
	preview.Connected = c.connected
	preview.Hidden = hide
	c.lastPreview = preview

	if !c.connected || c.conn == nil {
		return preview
	}

	keyBytes, _ := json.Marshal(activity)
	key := fmt.Sprintf("%v:%s", hide, string(keyBytes))
	if key == c.lastSentKey && time.Since(c.lastSentAt) < 15*time.Second {
		return preview
	}

	c.nonce++
	args := map[string]any{
		"pid": os.Getpid(),
	}
	if !hide {
		args["activity"] = activity
	} else {
		args["activity"] = nil
	}

	frame := map[string]any{
		"cmd":   "SET_ACTIVITY",
		"args":  args,
		"nonce": fmt.Sprintf("%d", c.nonce),
	}

	if err := writePacket(c.conn, opFrame, frame); err != nil {
		c.log(fmt.Sprintf("Discord RPC send error: %v", err))
		c.closeLocked()
		return preview
	}

	_, _, _ = readPacket(c.conn)
	c.lastSentKey = key
	c.lastSentAt = time.Now()
	return preview
}

func (c *Client) buildActivityLocked(st bambu.PrinterState) (PresencePreview, map[string]any, bool) {
	cfg := c.cfg
	gcode := strings.ToUpper(st.GcodeState)

	largeImage := bambu.DefaultBrandImageURL
	if cfg.ShowSpecificPrinter && st.Model.ImageURL != "" {
		largeImage = st.Model.ImageURL
	}
	largeText := st.Model.Name
	if largeText == "" {
		largeText = "Bambu Lab Printer"
	}

	var details, stateText, smallImage, smallText, timerLabel string
	var startTs, endTs int64

	completionWindow := time.Duration(cfg.CompletionMinutes) * time.Minute
	if completionWindow <= 0 {
		completionWindow = 5 * time.Minute
	}

	if len(st.Errors) > 0 && cfg.ShowErrors {
		details = fmt.Sprintf("Printer Alert (%s)", st.Errors[0].Code)
		if st.SubtaskName != "" && cfg.ShowFilename {
			details += " | " + st.SubtaskName
		}
		stateText = st.Errors[0].Message
		smallImage = "https://img.icons8.com/color/128/error--v1.png"
		if len(st.Errors) > 1 {
			smallText = fmt.Sprintf("%d HMS Errors", len(st.Errors))
		} else {
			smallText = "HMS Error"
		}
	} else if gcode == "PAUSE" && st.IsAlertStage() && cfg.ShowErrors {
		details = "Print Paused (Alert)"
		if st.StageID == 6 {
			details = "Filament Runout!"
		}
		if cfg.ShowFilename && st.SubtaskName != "" {
			details += " | " + st.SubtaskName
		}
		stateText = st.StageText
		smallImage = "https://img.icons8.com/color/128/error--v1.png"
		smallText = "Printer Alert"
	} else {
		switch gcode {
		case "RUNNING":
			c.completionTime = time.Time{}
			if c.printStartTime.IsZero() {
				c.printStartTime = time.Now()
			}

			isPreparing := st.StageID != 0 && st.StageText != ""
			if isPreparing {
				details = fmt.Sprintf("%s: %d%%", st.StageText, st.Percent)
				smallImage = "https://img.icons8.com/color/128/settings--v1.png"
				smallText = st.StageText
			} else {
				details = fmt.Sprintf("Printing: %d%%", st.Percent)
				smallImage = "https://img.icons8.com/color/128/3d-printer.png"
				smallText = "Printing"
				if cfg.ShowFilament {
					if fil := st.ActiveFilamentLabel(); fil != "" {
						smallText = fil
					}
				}
			}

			if cfg.ShowFilename && st.SubtaskName != "" {
				details += " | " + st.SubtaskName
			}

			var stateParts []string
			if cfg.ShowLayers && st.TotalLayers > 0 {
				stateParts = append(stateParts, fmt.Sprintf("Layer %d/%d", st.Layer, st.TotalLayers))
			}
			if cfg.ShowTime && st.RemainingMins > 0 {
				stateParts = append(stateParts, formatDurationMins(st.RemainingMins)+" left")
			}
			if cfg.ShowFilament {
				if fil := st.ActiveFilamentLabel(); fil != "" {
					stateParts = append(stateParts, fil)
				}
			}
			if cfg.ShowSpeed && st.SpeedName != "" {
				stateParts = append(stateParts, fmt.Sprintf("%s (%d%%)", st.SpeedName, st.SpeedMag))
			}
			stateText = strings.Join(stateParts, " | ")
			if stateText == "" {
				stateText = "Printing in progress"
			}

			if cfg.UseETACountdown && st.RemainingMins > 0 {
				endTs = time.Now().Add(time.Duration(st.RemainingMins) * time.Minute).Unix()
				timerLabel = fmt.Sprintf("ETA %s", time.Unix(endTs, 0).Format("3:04 PM"))
			} else {
				startTs = c.printStartTime.Unix()
				timerLabel = fmt.Sprintf("Elapsed since %s", c.printStartTime.Format("3:04 PM"))
			}

			if strings.TrimSpace(cfg.CustomDetailsFmt) != "" {
				details = applyTemplate(cfg.CustomDetailsFmt, st)
			}
			if strings.TrimSpace(cfg.CustomStateFmt) != "" {
				stateText = applyTemplate(cfg.CustomStateFmt, st)
			}

		case "FINISH":
			c.printStartTime = time.Time{}
			if c.completionTime.IsZero() {
				c.completionTime = time.Now()
			}
			details = "Print Completed"
			if cfg.ShowFilename && st.SubtaskName != "" {
				details += " | " + st.SubtaskName
			}
			stateText = "Waiting for user"
			smallImage = "https://img.icons8.com/color/128/ok--v1.png"
			smallText = "Complete"

		case "PAUSE":
			details = "Print Paused"
			if cfg.ShowFilename && st.SubtaskName != "" {
				details += " | " + st.SubtaskName
			}
			if st.StageText != "" {
				stateText = st.StageText
			} else {
				stateText = "Waiting to resume"
			}
			smallImage = "https://img.icons8.com/color/128/pause--v1.png"
			smallText = "Paused"

		case "FAILED":
			c.printStartTime = time.Time{}
			details = "Print Failed"
			if cfg.ShowFilename && st.SubtaskName != "" {
				details += " | " + st.SubtaskName
			}
			stateText = "Check printer"
			smallImage = "https://img.icons8.com/color/128/error--v1.png"
			smallText = "Failed"

		case "IDLE", "PREPARE":
			c.printStartTime = time.Time{}
			if !c.completionTime.IsZero() && time.Since(c.completionTime) < completionWindow {
				details = "Print Completed"
				stateText = "Recently finished a print"
				smallImage = "https://img.icons8.com/color/128/ok--v1.png"
				smallText = "Complete"
			} else {
				c.completionTime = time.Time{}
				if gcode == "PREPARE" {
					details = "Preparing"
					if cfg.ShowFilename && st.SubtaskName != "" {
						details += ": " + st.SubtaskName
					}
					if st.StageText != "" {
						stateText = st.StageText
					} else {
						stateText = "Starting up..."
					}
					smallImage = "https://img.icons8.com/color/128/settings--v1.png"
					smallText = "Preparing"
				} else {
					if cfg.HideWhenIdle {
						return PresencePreview{
							Details: "Hidden (Printer Idle)",
							State:   "Presence cleared",
						}, nil, true
					}
					details = "Idling"
					stateText = "Ready to print"
					smallImage = "https://img.icons8.com/color/128/ok--v1.png"
					smallText = "Idle"
				}
			}

		default:
			c.printStartTime = time.Time{}
			details = fmt.Sprintf("Status: %s", st.GcodeState)
			stateText = "Printer is online"
			smallImage = "https://img.icons8.com/color/128/ok--v1.png"
			smallText = "Online"
		}
	}

	if cfg.ShowTemps {
		tempStr := fmt.Sprintf("Nozzle: %dC | Bed: %dC", int(math.Round(st.NozzleTemp)), int(math.Round(st.BedTemp)))
		if st.ChamberTemp > 0 {
			tempStr += fmt.Sprintf(" | Chamber: %dC", int(math.Round(st.ChamberTemp)))
		}
		largeText += " | " + tempStr
	}

	details = truncate(details, 128)
	stateText = truncate(stateText, 128)
	largeText = truncate(largeText, 128)
	smallText = truncate(smallText, 128)

	assets := map[string]any{
		"large_image": largeImage,
		"large_text":  largeText,
	}
	if smallImage != "" {
		assets["small_image"] = smallImage
		assets["small_text"] = smallText
	}

	activity := map[string]any{
		"details": details,
		"state":   stateText,
		"assets":  assets,
	}

	if endTs > 0 {
		activity["timestamps"] = map[string]any{
			"end": endTs,
		}
	} else if startTs > 0 {
		activity["timestamps"] = map[string]any{
			"start": startTs,
		}
	}

	preview := PresencePreview{
		Details:    details,
		State:      stateText,
		LargeImage: largeImage,
		LargeText:  largeText,
		SmallImage: smallImage,
		SmallText:  smallText,
		TimerLabel: timerLabel,
	}

	return preview, activity, false
}

func applyTemplate(tpl string, st bambu.PrinterState) string {
	r := strings.NewReplacer(
		"{percent}", fmt.Sprintf("%d%%", st.Percent),
		"{file}", st.SubtaskName,
		"{layer}", fmt.Sprintf("%d", st.Layer),
		"{total_layers}", fmt.Sprintf("%d", st.TotalLayers),
		"{eta}", formatDurationMins(st.RemainingMins),
		"{stage}", st.StageText,
		"{nozzle}", fmt.Sprintf("%dC", int(math.Round(st.NozzleTemp))),
		"{bed}", fmt.Sprintf("%dC", int(math.Round(st.BedTemp))),
		"{chamber}", fmt.Sprintf("%dC", int(math.Round(st.ChamberTemp))),
		"{filament}", st.ActiveFilamentLabel(),
		"{speed}", st.SpeedName,
	)
	return r.Replace(tpl)
}

func formatDurationMins(mins int) string {
	if mins <= 0 {
		return "0m"
	}
	h := mins / 60
	m := mins % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func (c *Client) log(msg string) {
	if c.onLog != nil {
		c.onLog(msg)
	}
}

func writePacket(conn net.Conn, opcode uint32, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.LittleEndian, opcode)
	_ = binary.Write(buf, binary.LittleEndian, uint32(len(data)))
	buf.Write(data)
	_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	_, err = conn.Write(buf.Bytes())
	return err
}

func readPacket(conn net.Conn) (uint32, []byte, error) {
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	header := make([]byte, 8)
	if _, err := io.ReadFull(conn, header); err != nil {
		return 0, nil, err
	}
	opcode := binary.LittleEndian.Uint32(header[0:4])
	length := binary.LittleEndian.Uint32(header[4:8])
	if length > 1024*1024 {
		return 0, nil, fmt.Errorf("packet too large: %d", length)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(conn, body); err != nil {
		return 0, nil, err
	}
	return opcode, body, nil
}
