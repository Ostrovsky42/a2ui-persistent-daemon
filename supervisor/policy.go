package supervisor

import "fmt"

type ViewerPolicy string

const (
	ViewerAuto   ViewerPolicy = "auto"
	ViewerNever  ViewerPolicy = "never"
	ViewerAlways ViewerPolicy = "always"
)

func ParseViewerPolicy(value string) (ViewerPolicy, error) {
	switch value {
	case "", "auto":
		return ViewerAuto, nil
	case "never":
		return ViewerNever, nil
	case "always":
		return ViewerAlways, nil
	default:
		return "", fmt.Errorf("invalid viewer policy %q", value)
	}
}

func (p ViewerPolicy) Allows(_ func(string) string) bool {
	return true
}
