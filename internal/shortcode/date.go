package shortcode

import (
	"fmt"
	"strings"
	"time"
)

// now is the clock the date variable reads; tests replace it.
var now = time.Now

// months are the month names by language, January first.
var months = map[string][12]string{
	"en": {"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
	"de": {"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"},
	"fr": {"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"},
	"es": {"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"},
	"it": {"gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno", "luglio", "agosto", "settembre", "ottobre", "novembre", "dicembre"},
	"nl": {"januari", "februari", "maart", "april", "mei", "juni", "juli", "augustus", "september", "oktober", "november", "december"},
	"pt": {"janeiro", "fevereiro", "março", "abril", "maio", "junho", "julho", "agosto", "setembro", "outubro", "novembro", "dezembro"},
}

// FormatDate writes d in layout: "iso" or "" for YYYY-MM-DD, else a
// pattern of YYYY or YY (the year), MON (the month's name in lang), MM or
// M (its number, with or without a leading zero) and DD or D (the day of
// the month, likewise) between other characters. A language without
// month names gets the English ones.
func FormatDate(d time.Time, layout, lang string) string {
	if layout == "" || layout == "iso" {
		return d.Format(time.DateOnly)
	}
	names, ok := months[strings.ToLower(strings.SplitN(lang, "-", 2)[0])]
	if !ok {
		names = months["en"]
	}
	tokens := []struct {
		tok string
		val func() string
	}{
		{"YYYY", func() string { return fmt.Sprintf("%04d", d.Year()) }},
		{"YY", func() string { return fmt.Sprintf("%02d", d.Year()%100) }},
		{"MON", func() string { return names[d.Month()-1] }},
		{"MM", func() string { return fmt.Sprintf("%02d", int(d.Month())) }},
		{"M", func() string { return fmt.Sprint(int(d.Month())) }},
		{"DD", func() string { return fmt.Sprintf("%02d", d.Day()) }},
		{"D", func() string { return fmt.Sprint(d.Day()) }},
	}
	var b strings.Builder
next:
	for s := layout; s != ""; {
		for _, t := range tokens {
			if strings.HasPrefix(s, t.tok) {
				b.WriteString(t.val())
				s = s[len(t.tok):]
				continue next
			}
		}
		b.WriteByte(s[0])
		s = s[1:]
	}
	return b.String()
}

// date is the date variable: vars' date if it is one (YYYY-MM-DD), else
// today, written in vars' date_format. A date that is not YYYY-MM-DD is
// kept as written.
func date(vars map[string]string, lang string) string {
	d := now()
	if s, ok := vars["date"]; ok {
		t, err := time.Parse(time.DateOnly, s)
		if err != nil {
			return s
		}
		d = t
	}
	return FormatDate(d, vars["date_format"], lang)
}
