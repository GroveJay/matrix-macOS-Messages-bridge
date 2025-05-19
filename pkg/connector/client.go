package connector

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/GroveJay/matrix-macOS-Messages-bridge/pkg/macos"
	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/simplevent"
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
	m.UserLogin.Log.Info().Msgf("Added chat DB to fs watcher for userID %s", userID)

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
	m.UserLogin.Log.Info().Msgf("Gt maximum messages time for userID %s", userID)

	go func() {
		defer watcher.Close()
		err := m.watchMessagesDBFile(watcher, *initialMaxMessagesTimestamp)
		if err != nil {
			m.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateUnknownError,
				Error:      "macos-messages-watch-messages-error",
				Message:    fmt.Sprintf("Failed while watching messages db: %v", err),
				Info:       map[string]any{},
			})
		}
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
	return networkid.UserID(m.UserLogin.ID) == userID
}

func (m *MessagesClient) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	chatName, avatar, err := m.MacOSMessagesClient.GetChatDetails(portal.ID)
	if err != nil {
		m.UserLogin.Log.Error().Msgf("Failed to get chat details for group %s: %s", portal.ID, err)
		return nil, err
	}
	memberMap, err := m.MacOSMessagesClient.GetChatMemberMap(portal.ID, networkid.UserID(m.UserLogin.ID))
	if err != nil {
		m.UserLogin.Log.Error().Msgf("failed to get chat members for group %s: %s", portal.ID, err)
		return nil, err
	}

	memberMap[networkid.UserID(m.UserLogin.ID)] = bridgev2.ChatMember{
		Membership: event.MembershipJoin,
		EventSender: bridgev2.EventSender{
			IsFromMe: true,
		},
	}

	contactsMap, err := m.MacOSContactsClient.GetContactsMap()
	if err != nil {
		m.UserLogin.Log.Error().Msgf("failed to get contacts: %s", err)
		return nil, err
	}
	macos.SupplementMemberMapWithContactsMap(&memberMap, contactsMap, *m.MacOSContactsClient)

	return &bridgev2.ChatInfo{
		Name:   chatName,
		Avatar: avatar,
		Members: &bridgev2.ChatMemberList{
			IsFull:    true,
			MemberMap: memberMap,
		},
	}, nil
}

func (m *MessagesClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	return m.MacOSContactsClient.GetContactUserInfo(string(ghost.ID))
}

// HandleMatrixMessage implements bridgev2.NetworkAPI.
func (m *MessagesClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (message *bridgev2.MatrixMessageResponse, err error) {
	panic("unimplemented")
}

func (m *MessagesClient) watchMessagesDBFile(watcher *fsnotify.Watcher, maxMessagesTimestamp int64) error {
	var skipEvents bool
	var handleLock sync.Mutex
	nonSentMessages := make(map[string]bool)
	minReceiptTime := time.Now()
	for {
		select {
		case <-m.MessagesDBWatcherStopChannel:
			return nil
		case err := <-watcher.Errors:
			return fmt.Errorf("error in watcher: %w", err)
		case _, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if skipEvents {
				continue
			}

			skipEvents = true
			go func() {
				handleLock.Lock()
				defer handleLock.Unlock()

				if newMessages, err := m.MacOSMessagesClient.GetMessagesNewerThan(maxMessagesTimestamp); err != nil {
					m.UserLogin.Log.Warn().Msgf("Error reading messages after fsevent: %v", err)
				} else {
					for _, message := range newMessages {
						if message.Date > maxMessagesTimestamp {
							maxMessagesTimestamp = message.Date
						}

						if !message.IsSent {
							nonSentMessages[message.GUID] = true
						} else if _, ok := nonSentMessages[message.GUID]; ok {
							delete(nonSentMessages, message.GUID)
							continue
						}

						m.MessagesChannel <- message
					}
				}
				var latestReadReceipts []*macos.ReadReceipt
				var err error
				if latestReadReceipts, minReceiptTime, err = m.MacOSMessagesClient.GetReadReceiptsSince(minReceiptTime); err != nil {
					m.UserLogin.Log.Warn().Msgf("error reading receipts after fsevent: %v", err)
				} else {
					for _, readReceipt := range latestReadReceipts {
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

func (m *MessagesClient) PortalKeyFromMessage(message *macos.Message) networkid.PortalKey {
	return networkid.PortalKey{
		ID:       macos.MakeMessagesPortalID(m.UserLogin.ID, message.ChatGUID),
		Receiver: m.UserLogin.ID,
	}
}

func (m *MessagesClient) QueueRemoteEventWrapper(evt bridgev2.RemoteEvent) {
	if m.DryRun {
		// m.UserLogin.Log.Info().Msgf("would send event: %s", evt.GetType())
		if asMessageEvent, ok := evt.(*simplevent.Message[macos.Message]); ok {
			// m.UserLogin.Log.Info().Msgf("simpleEvent.Message: %s: %s", evt.GetType(), asMessageEvent.Data)
			context := context.TODO()
			portal := &bridgev2.Portal{
				Portal: &database.Portal{
					MXID: "foobar",
				},
			}

			if asMessageEvent.ConvertMessageFunc != nil {
				if len(asMessageEvent.Data.CombinedComponents) > 1 || asMessageEvent.Data.Subject != "" {
					m.UserLogin.Log.Info().Msgf("original:\n%s", asMessageEvent.Data)
					convertResult, err := asMessageEvent.ConvertMessageFunc(context, portal, &macos.MockMatrixAPI{}, asMessageEvent.Data)
					if err != nil {
						m.UserLogin.Log.Error().Msgf("error converting message: %v", err)
					}
					m.UserLogin.Log.Info().Msgf(macos.ConvertConvertedMessageToString(convertResult))
				}
			} else if asMessageEvent.ConvertEditFunc != nil {
				m.UserLogin.Log.Info().Msgf("[EDITED] original:\n%s", asMessageEvent.Data)
				convertResult, err := asMessageEvent.ConvertEditFunc(context, portal, &macos.MockMatrixAPI{}, []*database.Message{}, asMessageEvent.Data)
				if err != nil {
					m.UserLogin.Log.Error().Msgf("error converting message: %v", err)
				}
				m.UserLogin.Log.Info().Msgf(macos.ConvertEditToString(convertResult))
			} else if asReactionEvent, ok := evt.(*simplevent.ReactionSync); ok {
				m.UserLogin.Log.Info().Msgf("ReactionSync: %d reactions", len(asReactionEvent.Reactions.Users))
			}
		}

		return
	}
	m.UserLogin.Bridge.QueueRemoteEvent(m.UserLogin, evt)
}

func (m *MessagesClient) HandleTapback(message *macos.Message) {
	reactions := []*bridgev2.BackfillReaction{}

	if !message.Tapback.Remove {
		emoji := message.Tapback.GetEmoji()
		reactions = append(reactions, &bridgev2.BackfillReaction{
			Timestamp: message.CreatedAt,
			Emoji:     emoji,
			EmojiID:   networkid.EmojiID(emoji),
		})
	}

	m.QueueRemoteEventWrapper(&simplevent.ReactionSync{
		EventMeta: simplevent.EventMeta{
			Type: bridgev2.RemoteEventReactionSync,
			LogContext: func(c zerolog.Context) zerolog.Context {
				return c.Str("message_guid", message.GUID)
			},
			PortalKey: m.PortalKeyFromMessage(message),
		},
		TargetMessage: networkid.MessageID(message.Tapback.TargetGUID),
		Reactions: &bridgev2.ReactionSyncData{
			Users: map[networkid.UserID]*bridgev2.ReactionSyncUser{
				networkid.UserID(message.Sender.String()): {
					HasAllReactions: true,
					Reactions:       reactions,
				},
			},
		},
	})
}

func (m *MessagesClient) HandleRetraction(message *macos.Message) {
	m.QueueRemoteEventWrapper(&simplevent.MessageRemove{
		EventMeta: simplevent.EventMeta{
			Type: bridgev2.RemoteEventMessageRemove,
			LogContext: func(c zerolog.Context) zerolog.Context {
				return c.Str("message_guid", message.GUID)
			},
			PortalKey: m.PortalKeyFromMessage(message),
		},
		TargetMessage: networkid.MessageID(message.GUID),
		// OnlyForMe: true,
	})
}

func (m *MessagesClient) HandleEdit(message *macos.Message) {
	m.QueueRemoteEventWrapper(&simplevent.Message[macos.Message]{
		EventMeta: simplevent.EventMeta{
			Sender: bridgev2.EventSender{
				Sender:   networkid.UserID(message.Sender.LocalID),
				IsFromMe: message.IsFromMe,
			},
			Type: bridgev2.RemoteEventEdit,
			LogContext: func(c zerolog.Context) zerolog.Context {
				return c.Str("message_guid", message.GUID)
			},
			PortalKey:    m.PortalKeyFromMessage(message),
			CreatePortal: true,
			Timestamp:    time.Now(),
		},
		TargetMessage:   networkid.MessageID(message.GUID),
		ID:              networkid.MessageID(message.GUID),
		Data:            *message,
		ConvertEditFunc: ConvertEditMessage,
	})
}

func (m *MessagesClient) HandleNormalMessage(message *macos.Message) {
	m.QueueRemoteEventWrapper(&simplevent.Message[macos.Message]{
		EventMeta: simplevent.EventMeta{
			Sender: bridgev2.EventSender{
				Sender:   networkid.UserID(message.Sender.LocalID),
				IsFromMe: message.IsFromMe,
			},
			Type: bridgev2.RemoteEventMessage,
			LogContext: func(c zerolog.Context) zerolog.Context {
				return c.Str("message_guid", message.GUID)
			},
			PortalKey:    m.PortalKeyFromMessage(message),
			CreatePortal: true,
			Timestamp:    time.Now(),
		},
		ID:                 networkid.MessageID(message.GUID),
		ConvertMessageFunc: ConvertMessage,
		Data:               *message,
	})
}

func ConvertEditMessage(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, existing []*database.Message, data macos.Message) (*bridgev2.ConvertedEdit, error) {
	editParts, err := data.ConvertMessageToParts(ctx, intent, portal.MXID)
	if err != nil {
		return nil, fmt.Errorf("converting data message to parts: %w", err)
	}
	modifiedParts := []*bridgev2.ConvertedEditPart{}
	deletedParts := []*database.Message{}
	addedParts := &bridgev2.ConvertedMessage{
		Parts: []*bridgev2.ConvertedMessagePart{},
	}

	// This is almost certainly wrong, but who knows...
	for existingIndex := 0; existingIndex < len(editParts) && existingIndex < len(existing); existingIndex++ {
		modifiedParts = append(modifiedParts, editParts[existingIndex].ToEditPart(existing[existingIndex]))
	}
	if len(existing) < len(editParts) {
		addedParts.Parts = editParts[len(existing):]
	} else if len(editParts) < len(existing) {
		deletedParts = existing[len(editParts):]
	}

	return &bridgev2.ConvertedEdit{
		ModifiedParts: modifiedParts,
		DeletedParts:  deletedParts,
		AddedParts:    addedParts,
	}, nil
}

func ConvertMessage(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, data macos.Message) (*bridgev2.ConvertedMessage, error) {
	var replyTo *networkid.MessageOptionalPartID
	if data.ReplyToGUID != "" {
		replyTo = &networkid.MessageOptionalPartID{
			MessageID: networkid.MessageID(data.ReplyToGUID),
		}
	}
	parts, err := data.ConvertMessageToParts(ctx, intent, portal.MXID)
	if err != nil {
		return nil, fmt.Errorf("converting data message to parts: %w", err)
	}
	return &bridgev2.ConvertedMessage{
		ReplyTo: replyTo,
		Parts:   parts,
	}, nil
}

func (m *MessagesClient) HandleMessage(message *macos.Message) {
	if message.Tapback != nil {
		m.HandleTapback(message)
		return
	}
	if message.IsRetracted {
		m.HandleRetraction(message)
		return
	}
	if message.IsEdited {
		m.HandleEdit(message)
		return
	}
	m.HandleNormalMessage(message)
}

func (m *MessagesClient) QueueMemberChatInfoChange(portalKey networkid.PortalKey, messageGUID string, userID networkid.UserID, membership event.Membership) {
	m.QueueRemoteEventWrapper(&simplevent.ChatInfoChange{
		EventMeta: simplevent.EventMeta{
			Type: bridgev2.RemoteEventChatInfoChange,
			LogContext: func(c zerolog.Context) zerolog.Context {
				return c.Str("message_guid", messageGUID)
			},
			PortalKey: portalKey,
		},
		ChatInfoChange: &bridgev2.ChatInfoChange{
			MemberChanges: &bridgev2.ChatMemberList{
				MemberMap: map[networkid.UserID]bridgev2.ChatMember{
					userID: {
						EventSender: bridgev2.EventSender{
							SenderLogin: m.UserLogin.ID,
						},
						Membership: membership,
					},
				},
			},
		},
	})
}

func (m *MessagesClient) HandleMember(message *macos.Message) {
	membership := event.MembershipJoin
	if message.GroupActionType == 1 {
		membership = event.MembershipLeave
	}
	m.QueueMemberChatInfoChange(m.PortalKeyFromMessage(message), message.GUID, networkid.UserID(message.Target.LocalID), membership)
}

func (m *MessagesClient) HandleName(message *macos.Message) {
	m.QueueRemoteEventWrapper(&simplevent.ChatInfoChange{
		EventMeta: simplevent.EventMeta{
			Type: bridgev2.RemoteEventChatInfoChange,
			LogContext: func(c zerolog.Context) zerolog.Context {
				return c.Str("message_guid", message.GUID)
			},
			PortalKey: m.PortalKeyFromMessage(message),
		},
		ChatInfoChange: &bridgev2.ChatInfoChange{
			ChatInfo: &bridgev2.ChatInfo{
				Name: &message.NewGroupTitle,
			},
		},
	})
}

func (m *MessagesClient) HandleAvatarOrMemberLeave(message *macos.Message) {
	switch message.GroupActionType {
	case macos.GroupActionAddUser:
		m.QueueMemberChatInfoChange(m.PortalKeyFromMessage(message), message.GUID, networkid.UserID(message.Sender.LocalID), event.MembershipLeave)
	case macos.GroupActionSetAvatar:
		m.QueueRemoteEventWrapper(&simplevent.ChatInfoChange{
			EventMeta: simplevent.EventMeta{
				Type: bridgev2.RemoteEventChatInfoChange,
				LogContext: func(c zerolog.Context) zerolog.Context {
					return c.Str("message_guid", message.GUID)
				},
				PortalKey: m.PortalKeyFromMessage(message),
			},
			ChatInfoChange: &bridgev2.ChatInfoChange{
				ChatInfo: &bridgev2.ChatInfo{
					Avatar: &bridgev2.Avatar{
						ID: networkid.AvatarID(fmt.Sprintf("%s-avatar", message.GUID)),
						Get: func(ctx context.Context) (result []byte, err error) {
							if len(message.Attachments) < 1 {
								return nil, fmt.Errorf("no attachments found in update avatar message")
							}
							firstAttachment := message.Attachments[0]
							firstAttachment.PathOnDisk, err = macos.ReplaceHomeDirectory(firstAttachment.PathOnDisk)
							if err != nil {
								return nil, fmt.Errorf("getting avatar path: %w", err)
							}
							return os.ReadFile(firstAttachment.PathOnDisk)
						},
					},
				},
			},
		})
	case macos.GroupActionRemoveAvatar:
		m.QueueRemoteEventWrapper(&simplevent.ChatInfoChange{
			EventMeta: simplevent.EventMeta{
				Type: bridgev2.RemoteEventChatInfoChange,
				LogContext: func(c zerolog.Context) zerolog.Context {
					return c.Str("message_guid", message.GUID)
				},
				PortalKey: m.PortalKeyFromMessage(message),
			},
			ChatInfoChange: &bridgev2.ChatInfoChange{
				ChatInfo: &bridgev2.ChatInfo{
					Avatar: &bridgev2.Avatar{
						Remove: true,
					},
				},
			},
		})
	}
}

func (m *MessagesClient) HandleiMessage(message *macos.Message) error {
	switch message.ItemType {
	case macos.ItemTypeMessage:
		m.HandleMessage(message)
	case macos.ItemTypeMember:
		m.HandleMember(message)
	case macos.ItemTypeName:
		m.HandleName(message)
	case macos.ItemTypeAvatar:
		m.HandleAvatarOrMemberLeave(message)
	default:
		m.UserLogin.Log.Warn().Msgf("Skipping message [%s] of unknown type %d", message.GUID, message.ItemType)
	}
	return nil
}

func (m *MessagesClient) HandleiMessageReadReceipt(readReciept *macos.ReadReceipt) {
	m.UserLogin.Bridge.QueueRemoteEvent(m.UserLogin, &simplevent.Receipt{
		EventMeta: simplevent.EventMeta{
			Type:      bridgev2.RemoteEventReadReceipt,
			Timestamp: readReciept.ReadAt,
			PortalKey: networkid.PortalKey{
				ID: networkid.PortalID(readReciept.ChatGUID),
			},
			Sender: bridgev2.EventSender{
				IsFromMe:    readReciept.IsFromMe,
				SenderLogin: networkid.UserLoginID(readReciept.SenderGUID),
			},
		},
		LastTarget: networkid.MessageID(readReciept.ReadUpTo),
	})
}
