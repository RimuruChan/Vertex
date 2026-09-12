package dto

import contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"

type MedalConfig struct {
	Mode   string `json:"mode" enums:"none,count,percentage"`
	Gold   int    `json:"gold" minimum:"0" maximum:"100000"`
	Silver int    `json:"silver" minimum:"0" maximum:"100000"`
	Bronze int    `json:"bronze" minimum:"0" maximum:"100000"`
}

func (value *MedalConfig) Domain() *contestdomain.MedalConfig {
	if value == nil {
		return nil
	}
	return &contestdomain.MedalConfig{Mode: value.Mode, Gold: value.Gold, Silver: value.Silver, Bronze: value.Bronze}
}

func FromMedalConfig(value contestdomain.MedalConfig) *MedalConfig {
	value = contestdomain.MedalSettings(&value)
	return &MedalConfig{Mode: value.Mode, Gold: value.Gold, Silver: value.Silver, Bronze: value.Bronze}
}

type MedalSummary struct {
	Mode     string `json:"mode" enums:"count,percentage"`
	Eligible int    `json:"eligible"`
	Gold     int    `json:"gold"`
	Silver   int    `json:"silver"`
	Bronze   int    `json:"bronze"`
}

func FromMedalSummary(value *contestdomain.MedalSummary) *MedalSummary {
	if value == nil {
		return nil
	}
	return &MedalSummary{Mode: value.Mode, Eligible: value.Eligible, Gold: value.Gold, Silver: value.Silver, Bronze: value.Bronze}
}
