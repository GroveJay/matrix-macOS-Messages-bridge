package connector

import (
	"context"
	"fmt"
	"strconv"

	"go.mau.fi/util/configupgrade"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/commands"
	"maunium.net/go/mautrix/bridgev2/database"
)

const (
	SYNC_MESSAGES_BY_GUID_COMMAND = "sync-message-by-guid"
	SYNC_MESSAGES_BY_GUID_ARGS    = "<GUID>"
	SYNC_MESSAGES_BY_GUID_TEXT    = "Sync a specific message by its GUID"

	SYNC_LAST_DAYS_MESSAGES_COMMAND = "sync-last-days-messages"
	SYNC_LAST_DAYS_MESSAGES_ARGS    = "[Days]"
	SYNC_LAST_DAYS_MESSAGES_TEXT    = "Sync messages from the last number of days (default 1)"

	SYNC_USER_BY_ID_COMMAND = "sync-user-by-id"
	SYNC_USER_BY_ID_ARGS    = "<ID>"
	SYNC_USER_BY_ID_TEXT    = "Sync a user's information by their ID"
)

type MessagesConnector struct {
	Bridge *bridgev2.Bridge
	Usage  string
}

var _ bridgev2.NetworkConnector = (*MessagesConnector)(nil)

func (m *MessagesConnector) Init(b *bridgev2.Bridge) {
	m.Bridge = b
	m.Usage = fmt.Sprintf("Usage: `$cmdprefix %s %s`", SYNC_MESSAGES_BY_GUID_COMMAND, SYNC_MESSAGES_BY_GUID_ARGS)
	m.Bridge.Commands.(*commands.Processor).AddHandler(&commands.FullHandler{
		Func: m.SyncMessageByGUID,
		Name: SYNC_MESSAGES_BY_GUID_COMMAND,
		Help: commands.HelpMeta{
			Section:     commands.HelpSectionChats,
			Description: SYNC_MESSAGES_BY_GUID_TEXT,
			Args:        SYNC_MESSAGES_BY_GUID_ARGS,
		},
		RequiresAdmin:           true,
		RequiresLogin:           true,
		RequiresLoginPermission: true,
	})
	m.Bridge.Commands.(*commands.Processor).AddHandler(&commands.FullHandler{
		Func: m.SyncLastDaysMessages,
		Name: SYNC_LAST_DAYS_MESSAGES_COMMAND,
		Help: commands.HelpMeta{
			Section:     commands.HelpSectionChats,
			Description: SYNC_LAST_DAYS_MESSAGES_TEXT,
			Args:        SYNC_LAST_DAYS_MESSAGES_ARGS,
		},
		RequiresAdmin:           true,
		RequiresLogin:           true,
		RequiresLoginPermission: true,
	})
	m.Bridge.Commands.(*commands.Processor).AddHandler(&commands.FullHandler{
		Func: m.SyncUserByID,
		Name: SYNC_USER_BY_ID_COMMAND,
		Help: commands.HelpMeta{
			Section:     commands.HelpSectionChats,
			Description: SYNC_USER_BY_ID_TEXT,
			Args:        SYNC_USER_BY_ID_ARGS,
		},
		RequiresAdmin:           true,
		RequiresLogin:           true,
		RequiresLoginPermission: true,
	})
}

func (m *MessagesConnector) Start(context.Context) error {
	m.Bridge.Log.Info().Msg("Start")
	return nil
}

func (m *MessagesConnector) GetCapabilities() *bridgev2.NetworkGeneralCapabilities {
	return &bridgev2.NetworkGeneralCapabilities{
		DisappearingMessages: true,
	}
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
		DefaultPort:      29441,
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

func (m *MessagesConnector) SyncMessageByGUID(ce *commands.Event) {
	login := ce.User.GetDefaultLogin()
	if login == nil {
		ce.Reply("Login not found")
		return
	}

	if len(ce.Args) != 1 {
		ce.Reply(m.Usage)
		return
	}

	mc := login.Client.(*MessagesClient)
	if err := mc.HandleSyncMessageByGUID(ce.Args[0]); err != nil {
		ce.Reply(fmt.Sprintf("Error syncing message by GUID: %v", err))
	}
	ce.React("✅")
}

func (m *MessagesConnector) SyncLastDaysMessages(ce *commands.Event) {
	login := ce.User.GetDefaultLogin()
	if login == nil {
		ce.Reply("Login not found")
		return
	}

	days := 1
	var err error
	if len(ce.Args) == 1 {
		days, err = strconv.Atoi(ce.Args[0])
		if err != nil {
			ce.Reply(fmt.Sprintf("Days input could not be parsed as number: %v", err))
		}
	} else if len(ce.Args) > 1 {
		ce.Reply(m.Usage)
		return
	}

	mc := login.Client.(*MessagesClient)
	if err := mc.HandleSyncMessagesByDays(days); err != nil {
		ce.Reply(fmt.Sprintf("Error syncing messages for past %d days: %v", days, err))
	}
	ce.React("✅")
}

func (m *MessagesConnector) SyncUserByID(ce *commands.Event) {
	login := ce.User.GetDefaultLogin()
	if login == nil {
		ce.Reply("Login not found")
		return
	}

	if len(ce.Args) != 1 {
		ce.Reply(m.Usage)
		return
	}

	mc := login.Client.(*MessagesClient)
	if err := mc.HandleSyncUserByID(ce.Ctx, ce.Args[0]); err != nil {
		ce.Reply(fmt.Sprintf("Error syncing user by guid: %v", err))
	}
	ce.React("✅")
}
