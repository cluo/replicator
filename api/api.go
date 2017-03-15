package api

import (
	"regexp"
)

// FindIP will return the IP address from a string. This is used to deal with
// responses from the Nomad API that contain the port such as 127.0.0.1:4646.
func FindIP(input string) string {
	numBlock := "(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])"
	regexPattern := numBlock + "\\." + numBlock + "\\." + numBlock + "\\." + numBlock

	regEx := regexp.MustCompile(regexPattern)
	return regEx.FindString(input)
}

// Max returns the largest float from a variable length list of floats.
func Max(values ...float64) float64 {
	max := values[0]
	for _, i := range values[1:] {
		if i > max {
			max = i
		}
	}

	return max
}

// Min returns the smallest float from a variable length list of floats.
func Min(values ...float64) float64 {
	min := values[0]
	for _, i := range values[1:] {
		if i < min {
			min = i
		}
	}
	return min
}
