package connector

import (
	"context"

	"go.mau.fi/util/configupgrade"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
)

type MessagesConnector struct {
	br *bridgev2.Bridge
}

var _ bridgev2.NetworkConnector = (*MessagesConnector)(nil)

func (m *MessagesConnector) Init(b *bridgev2.Bridge) {
	m.br = b
}

func (m *MessagesConnector) Start(context.Context) error {
	m.br.Log.Info().Msg("Start")
	return nil
}

func (m *MessagesConnector) GetCapabilities() *bridgev2.NetworkGeneralCapabilities {
	return &bridgev2.NetworkGeneralCapabilities{}
}

func (m *MessagesConnector) GetBridgeInfoVersion() (info int, capabilities int) {
	return 1, 1
}

func (m *MessagesConnector) GetName() bridgev2.BridgeName {
	return bridgev2.BridgeName{
		DisplayName:      "Messages macOS",
		NetworkURL:       "https://support.apple.com/guide/messages/welcome/mac",
		NetworkIcon:      "mxc://maunium.net/tManJEpANASZvDVzvRvhILdX",
		NetworkID:        "Messages",
		BeeperBridgeType: "github.com/GroveJay/matrix-macOS-Messages-bridge",
		DefaultPort:      29331,
	}
}

func (m *MessagesConnector) GetConfig() (example string, data any, upgrader configupgrade.Upgrader) {
	return "", nil, configupgrade.NoopUpgrader
}

func (m *MessagesConnector) GetDBMetaTypes() database.MetaTypes {
	return database.MetaTypes{
		Portal:   nil,
		Ghost:    nil,
		Message:  nil,
		Reaction: nil,
		UserLogin: func() any {
			return &UserLoginMetadata{}
		},
	}
}

type UserLoginMetadata struct {
	UserID string `json:"user_id"`
}

func (m *MessagesConnector) LoadUserLogin(ctx context.Context, login *bridgev2.UserLogin) (err error) {
	login.Log.Info().Msgf("MessagesConnector.LoadUserLogin")
	login.Client = &MessagesClient{
		UserLogin: login,
	}
	return nil
}
