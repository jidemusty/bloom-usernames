package username

import (
	"errors"
	"regexp"
	"strings"
)

var validName = regexp.MustCompile(`^[a-z0-9_]{3,20}$`)

func Normalize(input string) (string, error) {
	cleaned := strings.TrimSpace(input)
	cleaned = strings.ToLower(cleaned)

	if !validName.MatchString(cleaned) {
		return "", errors.New("username must be 3-20 characters: a-z, 0-9, _")
	}

	return cleaned, nil
}
