package domain

// ProblemLabel identifies a slot only inside one contest, independently of the
// reusable problem's UUID and tenant-local number.
type ProblemLabel string

func ParseProblemLabel(value string) (ProblemLabel, error) {
	if len(value) == 0 || len(value) > 8 {
		return "", Invalid("invalid contest problem label")
	}
	for i, c := range []byte(value) {
		letter := c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
		if !letter && (i == 0 || c < '0' || c > '9') {
			return "", Invalid("invalid contest problem label")
		}
	}
	return ProblemLabel(value), nil
}
