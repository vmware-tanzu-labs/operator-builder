package utils

import (
	"strings"

	pluralize "github.com/gertd/go-pluralize"
)

// ToPascalCase will convert a kebab-case string to a PascalCase name appropriate to
// use as a go variable name.
func ToPascalCase(name string) string {
	var output string

	makeUpper := true

	for _, letter := range name {
		if makeUpper {
			output += strings.ToUpper(string(letter))
			makeUpper = false
		} else {
			if letter == '-' {
				makeUpper = true
			} else {
				output += string(letter)
			}
		}
	}

	return output
}

// ToFileName will convert a kebab-case string to a snake_case name appropriate to
// use in a go filename.
func ToFileName(name string) string {
	return strings.ToLower(strings.Replace(name, "-", "_", -1))
}

// ToPackageName will convert a kebab-case string to an alllower name
// appropriate for directory and package names.
func ToPackageName(name string) string {
	return strings.ToLower(strings.Replace(name, "-", "", -1))
}

// PluralizeKind returns the plural version of a kind in lowercase.
func PluralizeKind(kind string) string {
	p := pluralize.NewClient()

	return strings.ToLower(p.Plural(kind))
}
