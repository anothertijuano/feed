// Command harbour-feed is the launcher binary for the feed. Sailfish
// client. It runs sailfish-qml against the installed QML so the launch
// path matches any other Silica app; output is appended to
// /tmp/harbour-feed.log for debugging.
package main

import (
	"os"
	"os/exec"
)

func main() {
	log, err := os.OpenFile("/tmp/harbour-feed.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log = os.Stderr
	}
	cmd := exec.Command("/usr/bin/sailfish-qml",
		"/usr/share/harbour-feed/qml/harbour-feed.qml")
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Run(); err != nil {
		os.Stderr.WriteString("harbour-feed: " + err.Error() + "\n")
		os.Exit(1)
	}
}
