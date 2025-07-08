package connector

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GroveJay/matrix-macOS-Messages-bridge/pkg/macos"
	"github.com/fsnotify/fsnotify"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/event"
)

type MessagesClient struct {
	UserLogin                    *bridgev2.UserLogin
	MacOSMessagesClient          *macos.MacOSMessagesClient
	MacOSContactsClient          *macos.MacOSContactsClient
	MessagesDBWatcherStopChannel chan struct{}
	MessagesChannel              chan *macos.Message
	ReadReceiptsChannel          chan *macos.ReadReceipt
	HandleMessagesStopChannel    chan struct{}
	DryRun                       bool
}

var _ bridgev2.NetworkAPI = (*MessagesClient)(nil)

func (m *MessagesClient) Connect(ctx context.Context) {
	var err error
	meta := m.UserLogin.Metadata.(*UserLoginMetadata)
	userID := meta.UserID
	m.UserLogin.Log.Info().Msgf("Starting login for userID %s", userID)
	if m.MacOSMessagesClient, err = macos.GetMessagesClient(userID, &m.UserLogin.Log); err != nil {
		m.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateBadCredentials,
			Error:      "macos-messages-connect-messages-client",
			Message:    fmt.Sprintf("Failed to create messages connection: %v", err),
			Info:       map[string]any{},
		})
		return
	}
	m.UserLogin.Log.Info().Msgf("Got Messages Client for userID %s", userID)

	if m.MacOSContactsClient, err = macos.GetContactsClient(userID); err != nil {
		m.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateBadCredentials,
			Error:      "macos-messages-connect-contacts-client",
			Message:    fmt.Sprintf("Failed to create contacts connection: %v", err),
			Info:       map[string]any{},
		})
		return
	}
	m.UserLogin.Log.Info().Msgf("Got Contacts Client for userID %s", userID)

	if err := m.MacOSContactsClient.ValidateConnection(); err != nil {
		m.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateBadCredentials,
			Error:      "macos-messages-connect-contacts-error",
			Message:    "Failed to validate contacts connection",
			Info:       map[string]any{},
		})
		return
	}
	m.UserLogin.Log.Info().Msgf("Validated Contacts Client for userID %s", userID)

	if err := m.MacOSMessagesClient.ValidateConnection(); err != nil {
		m.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateBadCredentials,
			Error:      "macos-messages-connect-messages-error",
			Message:    "Failed to validate messages connection",
			Info:       map[string]any{},
		})
		return
	}
	m.UserLogin.Log.Info().Msgf("Validated Messages Client for userID %s", userID)

	m.MessagesDBWatcherStopChannel = make(chan struct{}, 1)
	m.HandleMessagesStopChannel = make(chan struct{}, 1)
	m.MessagesChannel = make(chan *macos.Message)
	m.ReadReceiptsChannel = make(chan *macos.ReadReceipt)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		m.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateUnknownError,
			Error:      "macos-messages-filewatcher-create-error",
			Message:    fmt.Sprintf("failed to create fsnotify watcher: %v", err),
			Info:       map[string]any{},
		})
		return
	}
	m.UserLogin.Log.Info().Msgf("Created fs watcher for userID %s", userID)

	err = watcher.Add(filepath.Dir(m.MacOSMessagesClient.GetChatDBPath()))
	if err != nil {
		m.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateUnknownError,
			Error:      "macos-messages-filewatcher-add-error",
			Message:    fmt.Sprintf("failed to add chat DB to fsnotify watcher: %v", err),
			Info:       map[string]any{},
		})
		return
	}
	err = watcher.Add(filepath.Dir(m.MacOSMessagesClient.GetChatDBWALPath()))
	if err != nil {
		m.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateUnknownError,
			Error:      "macos-messages-filewatcher-add-error",
			Message:    fmt.Sprintf("failed to add chat DB WAL to fsnotify watcher: %v", err),
			Info:       map[string]any{},
		})
		return
	}
	m.UserLogin.Log.Info().Msgf("Added chat DB and WAL to fs watcher for userID %s", userID)

	initialMaxMessagesTimestamp, err := m.MacOSMessagesClient.GetMaxMessagesTime()
	if err != nil {
		m.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateUnknownError,
			Error:      "macos-messages-max-messages-time-error",
			Message:    fmt.Sprintf("failed to get maximum messages time: %v", err),
			Info:       map[string]any{},
		})
		return
	}
	m.UserLogin.Log.Info().Msgf("Got maximum messages time for userID %s", userID)

	go func() {
		defer watcher.Close()
		err := m.watchMessagesDBFile(watcher, *initialMaxMessagesTimestamp)
		if err != nil {
			m.UserLogin.Log.Error().Msgf("error returned from db watcher function: %v", err)
			m.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateUnknownError,
				Error:      "macos-messages-watch-messages-error",
				Message:    fmt.Sprintf("Failed while watching messages db: %v", err),
				Info:       map[string]any{},
			})
		}
		m.UserLogin.Log.Warn().Msgf("db watcher function returned without error (possible shutdown initiated)")
	}()

	go m.handleMessagesLoop()
	m.UserLogin.Log.Info().Msgf("Started handle message loop and db fs watcher for userID %s", userID)
}

func (m *MessagesClient) Disconnect() {
	m.UserLogin.Log.Info().Msgf("Disconnecting login for userID %s", m.UserLogin.ID)
	m.MessagesDBWatcherStopChannel <- struct{}{}
	m.HandleMessagesStopChannel <- struct{}{}
}

func (m *MessagesClient) IsLoggedIn() bool {
	return true
}

func (m *MessagesClient) LogoutRemote(ctx context.Context) {}

func (m *MessagesClient) GetCapabilities(ctx context.Context, portal *bridgev2.Portal) *event.RoomFeatures {
	return &event.RoomFeatures{}
}

func (m *MessagesClient) IsThisUser(ctx context.Context, userID networkid.UserID) bool {
	m.UserLogin.Log.Debug().Msgf("[IsThisUser] %s vs MessagesClient.UserLogin.ID %s", userID, m.UserLogin.ID)
	return networkid.UserID(m.UserLogin.ID) == userID
}

func (m *MessagesClient) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	m.UserLogin.Log.Debug().Msgf("[GetChatInfo] portalID: %s", portal.ID)
	chatName, avatar, err := m.MacOSMessagesClient.GetChatDetails(portal.ID)
	if err != nil {
		m.UserLogin.Log.Error().Msgf("Failed to get chat details for group %s: %s", portal.ID, err)
		return nil, err
	}
	m.UserLogin.Log.Debug().Msgf("[GetChatInfo] chatName: %s", *chatName)
	memberMap, err := m.MacOSMessagesClient.GetChatMemberMap(portal.ID, networkid.UserID(m.UserLogin.ID))
	if err != nil {
		m.UserLogin.Log.Error().Msgf("failed to get chat members for group %s: %s", portal.ID, err)
		return nil, err
	}
	m.UserLogin.Log.Debug().Msgf("[GetChatInfo] memberMap length: %d", len(memberMap))

	contactsMap, err := m.MacOSContactsClient.GetContactsMap()
	if err != nil {
		m.UserLogin.Log.Error().Msgf("failed to get contacts: %s", err)
		return nil, err
	}
	m.UserLogin.Log.Debug().Msgf("[GetChatInfo] contactsMap length: %d", len(contactsMap))
	macos.SupplementMemberMapWithContactsMap(&memberMap, contactsMap, *m.MacOSContactsClient)
	m.UserLogin.Log.Debug().Msgf("[GetChatInfo] supplemented membmerMap with contactsMap")

	if *chatName == "" {
		memberNames := []string{}
		for key, chatMember := range memberMap {
			if !chatMember.EventSender.IsFromMe {
				if chatMember.UserInfo.Name != nil {
					memberNames = append(memberNames, *chatMember.UserInfo.Name)
				} else {
					memberNames = append(memberNames, string(key))
				}
			}
		}
		memberNamesJoined := strings.Join(memberNames, ", ")
		chatName = &memberNamesJoined
	}
	m.UserLogin.Log.Debug().Msgf("[GetChatInfo] final chatName: %s", *chatName)

	if len(memberMap) == 2 {
		m.UserLogin.Log.Debug().Msgf("[GetChatInfo] member map of 2, setting avatar")
		for key, chatMember := range memberMap {
			if !chatMember.EventSender.IsFromMe {
				if chatMember.UserInfo.Avatar != nil {
					m.UserLogin.Log.Debug().Msgf("[GetChatInfo] set avatar to avatar for %s", key)
					avatar = chatMember.UserInfo.Avatar
					break
				}
			}
		}
	}

	topic := fmt.Sprintf("MacOS Messages chat with %s", *chatName)
	return &bridgev2.ChatInfo{
		Name:   chatName,
		Topic:  &topic,
		Avatar: avatar,
		Members: &bridgev2.ChatMemberList{
			IsFull:    true,
			MemberMap: memberMap,
		},
	}, nil
}

func (m *MessagesClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	m.UserLogin.Log.Debug().Msgf("[GetUserInfo] ghost.ID: %s", ghost.ID)
	if userInfo, err := m.MacOSContactsClient.GetContactUserInfo(string(ghost.ID)); err != nil {
		m.UserLogin.Log.Error().Msgf("[GetUserInfo] [ghost.ID: %s]: %v", ghost.ID, err)
		return nil, err
	} else {
		return userInfo, nil
	}
}

func (m *MessagesClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (message *bridgev2.MatrixMessageResponse, err error) {
	if err := m.MacOSMessagesClient.SendMessage(msg); err != nil {
		return nil, err
	}
	return &bridgev2.MatrixMessageResponse{}, nil
}

func (m *MessagesClient) HandleSyncMessageByGUID(guid string) error {
	dbMessage, err := m.MacOSMessagesClient.GetMessageByGUID(guid)
	if err != nil {
		return err
	}
	convertedMesage, err := macos.ConvertDBMessage(*dbMessage, string(m.UserLogin.ID))
	if err != nil {
		return err
	}
	m.UserLogin.Log.Info().Msgf("Queued handling message for guid %s", guid)
	m.MessagesChannel <- convertedMesage
	return nil
}

func (m *MessagesClient) watchMessagesDBFile(watcher *fsnotify.Watcher, maxMessagesTimestamp int64) error {
	var skipEvents bool
	var handleLock sync.Mutex
	minReceiptTime := time.Now()
	for {
		m.UserLogin.Log.Debug().Msgf("watchMessagesDBFile loop starting")
		select {
		case <-m.MessagesDBWatcherStopChannel:
			return nil
		case err := <-watcher.Errors:
			return fmt.Errorf("error in watcher: %w", err)
		case _, ok := <-watcher.Events:
			if !ok {
				m.UserLogin.Log.Warn().Msgf("got not ok event from watcher")
				return nil
			}
			if skipEvents {
				m.UserLogin.Log.Debug().Msgf("currently skipping events as previous loop has not completed")
				continue
			}

			skipEvents = true
			go func() {
				handleLock.Lock()
				defer handleLock.Unlock()

				m.UserLogin.Log.Info().Msg("Sleeping for two seconds after getting fs event to allow for DB settling")
				time.Sleep(2 * time.Second)

				if newDBMessages, err := m.MacOSMessagesClient.GetMessagesNewerThan(maxMessagesTimestamp); err != nil {
					m.UserLogin.Log.Warn().Msgf("Error reading messages after fsevent: %v", err)
				} else {
					for _, dbMessage := range newDBMessages {
						if dbMessage.Date > maxMessagesTimestamp {
							maxMessagesTimestamp = dbMessage.Date
						}

						if !dbMessage.IsSent {
							m.UserLogin.Log.Debug().Msgf("message is not yet sent, skipping")
							continue
						}

						if dbMessage.ChatGUID == "" {
							m.UserLogin.Log.Warn().Msgf("[%d] Message found without associated chat id, skipping", dbMessage.RowID)
							continue
						}

						convertedMesage, err := macos.ConvertDBMessage(*dbMessage, string(m.UserLogin.ID))
						if err != nil {
							m.UserLogin.Log.Warn().Msgf("error converting db message: %v", err)
							continue
						}
						m.UserLogin.Log.Debug().Msgf("sending message to handler channel")
						m.MessagesChannel <- convertedMesage
					}
				}
				m.UserLogin.Log.Debug().Msgf("getting read reciepts after fsevent")
				var latestReadReceipts []*macos.ReadReceipt
				var err error
				if latestReadReceipts, minReceiptTime, err = m.MacOSMessagesClient.GetReadReceiptsSince(minReceiptTime); err != nil {
					m.UserLogin.Log.Warn().Msgf("error reading receipts after fsevent: %v", err)
				} else {
					for _, readReceipt := range latestReadReceipts {
						m.UserLogin.Log.Debug().Msgf("sending read reciept to handler channel")
						m.ReadReceiptsChannel <- readReceipt
					}
				}

				skipEvents = false
			}()
		}
	}
}

func (m *MessagesClient) handleMessagesLoop() {
	for {
		var start time.Time
		var thing string
		var err error
		m.UserLogin.Log.Debug().Msgf("handleMessagesLoop starting")
		select {
		case <-m.HandleMessagesStopChannel:
			m.UserLogin.Log.Debug().Msg("Stopping handle messages loop")
			return
		case message := <-m.MessagesChannel:
			start = time.Now()
			thing = "iMessage"
			err = m.HandleiMessage(message)
		case readReciept := <-m.ReadReceiptsChannel:
			start = time.Now()
			thing = "read reciept"
			m.HandleiMessageReadReceipt(readReciept)
		}

		m.UserLogin.Log.Debug().Msgf(
			"Handled %s in %s (queued: %dr/%dm)",
			thing, time.Since(start),
			len(m.ReadReceiptsChannel), len(m.MessagesChannel),
		)

		if err != nil {
			m.UserLogin.Log.Error().Msgf(
				"Error handling %s: %v", thing, err,
			)
		}
	}
}
