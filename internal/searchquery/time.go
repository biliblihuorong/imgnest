package searchquery

import (
	"errors"
	"strings"
	"time"
	_ "time/tzdata" // Reproducible IANA availability in minimal deployment images.
)

// Location rejects host-local and arbitrary offset labels; UTC and IANA aliases are explicit.
func Location(name string) (*time.Location, error) {
	if name == "" || name == "Local" || strings.Contains(name, "..") || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\\x00") {
		return nil, errors.New("INVALID_TIMEZONE")
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, errors.New("INVALID_TIMEZONE")
	}
	return loc, nil
}

// DayStart chooses the earliest valid instant of a local civil date, including
// midnight gaps/overlaps. It does not assume that a civil day is 24 hours long.
func DayStart(day string, loc *time.Location) (time.Time, error) {
	if !validDate(day) || loc == nil {
		return time.Time{}, errors.New("INVALID_DATE")
	}
	date, _ := time.Parse("2006-01-02", day)
	// All IANA UTC offsets and date-line changes fit this bounded window.
	left, right := date.Add(-36*time.Hour), date.Add(36*time.Hour)
	belongs := func(t time.Time) bool { return t.In(loc).Format("2006-01-02") == day }
	// Find the first interval entering this date, then the exact first second.
	previous := left
	for current := left; !current.After(right); current = current.Add(15 * time.Minute) {
		if belongs(current) {
			lo, hi := previous.Unix(), current.Unix()
			for lo < hi {
				mid := lo + (hi-lo)/2
				if belongs(time.Unix(mid, 0)) {
					hi = mid
				} else {
					lo = mid + 1
				}
			}
			result := time.Unix(lo, 0).UTC()
			if result.Year() < 1 || result.Year() > 9999 {
				return time.Time{}, errors.New("INVALID_DATE")
			}
			return result, nil
		}
		previous = current
	}
	return time.Time{}, errors.New("DATE_NOT_REPRESENTABLE")
}
