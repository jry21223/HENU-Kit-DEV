package database

import "strings"

// EscapeLike neutralises the LIKE metacharacters in a user-supplied search term
// so it matches literally.
//
// Binding the term as a parameter prevents SQL injection but not pattern
// injection: a typed % or _ still reaches LIKE as a wildcard. Searching an
// admin listing for "%" would otherwise match every row and, combined with a
// LIMIT, silently return an arbitrary slice presented as a filtered result.
//
// The backslash is escaped first so the escapes added afterwards are not
// themselves re-escaped. Callers must pair this with ESCAPE '\\'.
func EscapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

// LikeContains builds a bound, literal "contains" pattern for use with
// ESCAPE '\\'.
func LikeContains(value string) string {
	return "%" + EscapeLike(value) + "%"
}
