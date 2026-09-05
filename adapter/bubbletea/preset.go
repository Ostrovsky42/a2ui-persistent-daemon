package bubbletea

import (
	"fmt"

	"a2ui/layout"
)

// Preset controls renderer-local presentation only. It is deliberately not a
// protocol property: the same authoritative document can be projected through
// different presets without semantic mutation.
type Preset string

const (
	PresetMinimal   Preset = "minimal"
	PresetDashboard Preset = "dashboard"
	PresetDense     Preset = "dense"
)

func (p Preset) Valid() bool {
	switch p {
	case PresetMinimal, PresetDashboard, PresetDense:
		return true
	default:
		return false
	}
}

func ParsePreset(s string) (Preset, error) {
	p := Preset(s)
	if !p.Valid() {
		return "", fmt.Errorf("unknown A2UI renderer preset %q", s)
	}
	return p, nil
}

type presetProfile struct {
	DefaultGap       int
	CardPadding      int
	PanelPadding     int
	SectionPadding   int
	CardBorder       layout.Border
	PanelBorder      layout.Border
	SectionBorder    layout.Border
	ActionGap        int
	TableRowGap      int
	TitlePrefix      string
	SubtitlePrefix   string
	ProgressBarGlyph string
	ProgressEmpty    string
}

func profileFor(p Preset) presetProfile {
	switch p {
	case PresetDashboard:
		return presetProfile{
			DefaultGap: 1, CardPadding: 1, PanelPadding: 1, SectionPadding: 0,
			CardBorder: layout.BorderRounded, PanelBorder: layout.BorderNormal, SectionBorder: layout.BorderNone,
			ActionGap: 2, TableRowGap: 0, TitlePrefix: "◆ ", SubtitlePrefix: "• ", ProgressBarGlyph: "█", ProgressEmpty: "░",
		}
	case PresetDense:
		return presetProfile{
			DefaultGap: 0, CardPadding: 0, PanelPadding: 0, SectionPadding: 0,
			CardBorder: layout.BorderNormal, PanelBorder: layout.BorderNone, SectionBorder: layout.BorderNone,
			ActionGap: 1, TableRowGap: 0, TitlePrefix: "", SubtitlePrefix: "", ProgressBarGlyph: "▓", ProgressEmpty: "░",
		}
	default:
		return presetProfile{
			DefaultGap: 1, CardPadding: 0, PanelPadding: 0, SectionPadding: 0,
			CardBorder: layout.BorderNone, PanelBorder: layout.BorderNone, SectionBorder: layout.BorderNone,
			ActionGap: 2, TableRowGap: 0, TitlePrefix: "", SubtitlePrefix: "", ProgressBarGlyph: "█", ProgressEmpty: "░",
		}
	}
}
