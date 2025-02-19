package utils

import "regexp"

// Given the input, locate the first line that matches the given regular
// expression and add the additions after (or before) that line.  If no lines
// match the regular expression, return the input unmodified.
func InsertLines(input, additions []string, matcher *regexp.Regexp, isBefore bool) []string {
	var result []string
	for i, line := range input {
		if matcher.MatchString(line) {
			if isBefore {
				result = append(result, input[:i]...)
				result = append(result, additions...)
				result = append(result, input[i:]...)
			} else {
				result = append(result, input[:i+1]...)
				result = append(result, additions...)
				result = append(result, input[i+1:]...)
			}
			return result
		}
	}
	return input[:]
}
