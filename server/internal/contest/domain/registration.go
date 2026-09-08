package domain

import "time"

// RegistrationOpen only describes the self-service window. Membership,
// admission, staff roles and passwords are checked separately.
func (c Contest) RegistrationOpen(now time.Time) bool {
	return RegistrationError(c.AllowSelfRegistration, c.AllowLateRegistration, c.BeginAt, c.EndAt, now) == nil
}

func RegistrationError(self, late bool, begin, end, now time.Time) error {
	if !now.Before(end) {
		return ErrRegistrationClosed
	}
	if !self {
		return ErrSelfRegistrationDisabled
	}
	if !late && !now.Before(begin) {
		return ErrRegistrationClosed
	}
	return nil
}

// Omitted update fields retain the value read under the contest row lock.
func RegistrationSetting(input *bool, fallback bool) bool {
	if input != nil {
		return *input
	}
	return fallback
}
