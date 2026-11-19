package history

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"bambupresence/internal/bambu"
)

const DefaultHistoryPath = "print_history.json"

type Entry struct {
	ID              string    `json:"id"`
	FileName        string    `json:"file_name"`
	PrinterModel    string    `json:"printer_model"`
	Filament        string    `json:"filament"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	DurationSeconds int64     `json:"duration_seconds"`
	FinalPercent    int       `json:"final_percent"`
	FinalLayer      int       `json:"final_layer"`
	TotalLayers     int       `json:"total_layers"`
	Status          string    `json:"status"`
}

type Stats struct {
	TotalPrints     int
	CompletedPrints int
	FailedPrints    int
	TotalSeconds    int64
}

type activeJob struct {
	fileName     string
	printerModel string
	filament     string
	startedAt    time.Time
	maxPercent   int
	maxLayer     int
	totalLayers  int
}

type Manager struct {
	mu      sync.Mutex
	path    string
	entries []Entry
	current *activeJob
	onChange func()
}

func NewManager(path string, onChange func()) *Manager {
	m := &Manager{
		path:     path,
		onChange: onChange,
	}
	m.load()
	return m
}

func (m *Manager) load() {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return
	}
	var loaded []Entry
	if err := json.Unmarshal(data, &loaded); err == nil {
		m.entries = loaded
	}
}

func (m *Manager) saveLocked() {
	data, err := json.MarshalIndent(m.entries, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(m.path, data, 0644)
}

func (m *Manager) Observe(st bambu.PrinterState) {
	m.mu.Lock()
	changed := false
	gcode := strings.ToUpper(st.GcodeState)

	switch gcode {
	case "RUNNING", "PAUSE":
		name := st.SubtaskName
		if name == "" {
			name = "Unnamed Print"
		}
		if m.current == nil {
			m.current = &activeJob{
				fileName:     name,
				printerModel: st.Model.Name,
				filament:     st.ActiveFilamentLabel(),
				startedAt:    time.Now(),
				maxPercent:   st.Percent,
				maxLayer:     st.Layer,
				totalLayers:  st.TotalLayers,
			}
		} else {
			if st.SubtaskName != "" {
				m.current.fileName = st.SubtaskName
			}
			if st.Percent > m.current.maxPercent {
				m.current.maxPercent = st.Percent
			}
			if st.Layer > m.current.maxLayer {
				m.current.maxLayer = st.Layer
			}
			if st.TotalLayers > m.current.totalLayers {
				m.current.totalLayers = st.TotalLayers
			}
			if fil := st.ActiveFilamentLabel(); fil != "" {
				m.current.filament = fil
			}
		}

	case "FINISH":
		if m.current != nil {
			m.finalizeLocked("Completed", 100)
			changed = true
		}

	case "FAILED":
		if m.current != nil {
			m.finalizeLocked("Failed", m.current.maxPercent)
			changed = true
		}

	case "IDLE":
		if m.current != nil {
			status := "Cancelled"
			if m.current.maxPercent >= 99 {
				status = "Completed"
			}
			m.finalizeLocked(status, m.current.maxPercent)
			changed = true
		}
	}

	m.mu.Unlock()
	if changed && m.onChange != nil {
		m.onChange()
	}
}

func (m *Manager) finalizeLocked(status string, finalPct int) {
	now := time.Now()
	dur := int64(now.Sub(m.current.startedAt).Seconds())
	if dur < 1 {
		dur = 1
	}
	entry := Entry{
		ID:              now.Format("20060102150405"),
		FileName:        m.current.fileName,
		PrinterModel:    m.current.printerModel,
		Filament:        m.current.filament,
		StartedAt:       m.current.startedAt,
		EndedAt:         now,
		DurationSeconds: dur,
		FinalPercent:    finalPct,
		FinalLayer:      m.current.maxLayer,
		TotalLayers:     m.current.totalLayers,
		Status:          status,
	}
	m.entries = append([]Entry{entry}, m.entries...)
	if len(m.entries) > 500 {
		m.entries = m.entries[:500]
	}
	m.current = nil
	m.saveLocked()
}

func (m *Manager) Entries() []Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Entry, len(m.entries))
	copy(out, m.entries)
	return out
}

func (m *Manager) Stats() Stats {
	m.mu.Lock()
	defer m.mu.Unlock()
	var s Stats
	s.TotalPrints = len(m.entries)
	for _, e := range m.entries {
		s.TotalSeconds += e.DurationSeconds
		if e.Status == "Completed" {
			s.CompletedPrints++
		} else {
			s.FailedPrints++
		}
	}
	return s
}

func (m *Manager) Clear() {
	m.mu.Lock()
	m.entries = nil
	m.saveLocked()
	m.mu.Unlock()
	if m.onChange != nil {
		m.onChange()
	}
}
