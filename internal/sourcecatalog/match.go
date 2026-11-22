package sourcecatalog

import (
	"regexp"
	"strings"
)

var editionSuffix = regexp.MustCompile(`(?i)(live|现场|伴奏|instrumental|remix|dj|翻唱|cover|demo|纯音乐|acoustic|不插电)\s*版`)

func Normalize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r >= 0x3400 && r <= 0x9fff) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func NormalizeTitle(value string) string {
	return Normalize(editionSuffix.ReplaceAllString(value, "$1"))
}

func SameArtist(left, right string) bool {
	for _, a := range artistParts(left) {
		for _, b := range artistParts(right) {
			if sameArtistName(a, b) {
				return true
			}
		}
	}
	return false
}

func sameArtistName(left, right string) bool {
	a, b := Normalize(left), Normalize(right)
	if a == "" || b == "" {
		return false
	}
	return a != "" && a == b
}

func artistParts(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ',', '，', '/', '、', '&', '＆', ';', '；':
			return true
		default:
			return false
		}
	})
}

func VersionTags(value string) string {
	value = strings.ToLower(value)
	var tags []string
	for _, tag := range []string{"live", "现场", "伴奏", "instrumental", "remix", "dj", "翻唱", "cover", "demo", "纯音乐", "acoustic", "不插电"} {
		if strings.Contains(value, tag) {
			tags = append(tags, tag)
		}
	}
	return strings.Join(tags, ",")
}
