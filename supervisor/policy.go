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

func (p ViewerPolicy) Allows(getenv func(string) string) bool {
	switch p {
	case ViewerNever:
		return false
	case ViewerAlways:
		return true
	case ViewerAuto:
		if getenv == nil {
			return false
		}
		if getenv("CI") != "" || getenv("SSH_CONNECTION") != "" || getenv("SSH_TTY") != "" {
			return false
		}
		return getenv("DISPLAY") != "" || getenv("WAYLAND_DISPLAY") != ""
	default:
		return false
	}
}
