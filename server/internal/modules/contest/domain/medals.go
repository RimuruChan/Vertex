package domain

const (
	MedalsNone       = "none"
	MedalsCount      = "count"
	MedalsPercentage = "percentage"
)

type MedalConfig struct {
	Mode                 string
	Gold, Silver, Bronze int
}

type MedalSummary struct {
	Mode                           string
	Eligible, Gold, Silver, Bronze int
}

func DefaultMedals(format string) MedalConfig {
	if NormalizeFormat(format) == FormatICPC {
		return MedalConfig{Mode: MedalsPercentage, Gold: 10, Silver: 20, Bronze: 30}
	}
	return MedalConfig{Mode: MedalsNone}
}

func MedalSettings(value *MedalConfig) MedalConfig {
	if value == nil {
		return MedalConfig{Mode: MedalsNone}
	}
	result := *value
	if result.Mode == "" {
		result.Mode = MedalsNone
	}
	return result
}

func (c MedalConfig) Validate() error {
	if c.Mode != MedalsNone && c.Mode != MedalsCount && c.Mode != MedalsPercentage {
		return Invalid("invalid medal allocation mode")
	}
	for _, value := range []int{c.Gold, c.Silver, c.Bronze} {
		if value < 0 || value > 100000 {
			return Invalid("medal values must be integers between 0 and 100000")
		}
	}
	if c.Mode == MedalsPercentage && c.Gold+c.Silver+c.Bronze > 100 {
		return Invalid("medal percentages must total at most 100")
	}
	return nil
}

// AssignMedals uses the visible ranked rows. Hidden solves never increase the
// public denominator. Ties use the first eligible position of their rank group;
// a tie crossing a cumulative cutoff receives the higher medal.
func AssignMedals(rows []RankRow, config MedalConfig) *MedalSummary {
	for i := range rows {
		rows[i].Medal = ""
	}
	if config.Mode == "" || config.Mode == MedalsNone {
		return nil
	}
	result := &MedalSummary{Mode: config.Mode}
	for _, row := range rows {
		if row.Solved > 0 {
			result.Eligible++
		}
	}
	gold, silver, bronze := config.Gold, config.Silver, config.Bronze
	if config.Mode == MedalsPercentage {
		gold = result.Eligible * gold / 100
		silver = result.Eligible * silver / 100
		bronze = result.Eligible * bronze / 100
	}
	position, groupPosition, priorRank := 0, 0, -1
	for i := range rows {
		if rows[i].Solved <= 0 {
			continue
		}
		position++
		if rows[i].Rank != priorRank {
			groupPosition, priorRank = position, rows[i].Rank
		}
		switch {
		case groupPosition <= gold:
			rows[i].Medal = "gold"
			result.Gold++
		case groupPosition <= gold+silver:
			rows[i].Medal = "silver"
			result.Silver++
		case groupPosition <= gold+silver+bronze:
			rows[i].Medal = "bronze"
			result.Bronze++
		}
	}
	return result
}
