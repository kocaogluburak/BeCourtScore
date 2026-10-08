package score

import "fmt"

func normalizeSetFormat(setRule, finalSet string) (string, string, error) {
	if setRule == "" {
		setRule = "STANDARD_6"
	}
	if finalSet == "" {
		finalSet = "REGULAR"
	}
	switch setRule {
	case "STANDARD_6", "FAST4", "PRO_SET":
	default:
		return "", "", fmt.Errorf("%w: set rule", ErrInvalid)
	}
	switch finalSet {
	case "REGULAR", "MATCH_TIEBREAK":
	default:
		return "", "", fmt.Errorf("%w: final set", ErrInvalid)
	}
	return setRule, finalSet, nil
}
