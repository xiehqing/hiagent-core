//go:build !darwin

package notification

import (
	_ "embed"
)

//go:embed hiagent-icon-solo.png
var icon []byte

// Icon contains the embedded PNG icon data for desktop notifications.
var Icon any = icon
