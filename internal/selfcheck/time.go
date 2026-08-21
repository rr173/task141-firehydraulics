package selfcheck

import "time"

// timeT / timeParse keep the main selfcheck file free of a stray time import
// when only the base-time parse uses it.
type timeT = time.Time

func timeParse(s string) timeT {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic("selfcheck: bad base time " + s + ": " + err.Error())
	}
	return t
}
