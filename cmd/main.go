package main

import (
	"github.com/GroveJay/matrix-macOS-Messages-bridge/pkg/connector"
	"maunium.net/go/mautrix/bridgev2/matrix/mxmain"
)

var (
	Tag       = "unknown"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func main() {
	m := mxmain.BridgeMain{
		Name:        "jrgrover-messages",
		Description: "A MacOS Messages && Contacts matrix bridge",
		URL:         "",
		Version:     "0.1.0",
		Connector:   &connector.MessagesConnector{},
	}
	m.InitVersion(Tag, Commit, BuildTime)
	m.Run()
}
