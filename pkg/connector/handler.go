package connector

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/GroveJay/matrix-macOS-Messages-bridge/pkg/macos"
	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/simplevent"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func (m *MessagesClient) PortalKeyFromMessage(message *macos.Message) networkid.PortalKey {
	return networkid.PortalKey{
		ID:       macos.MakeMessagesPortalID(m.UserLogin.ID, message.ChatGUID),
		Receiver: m.UserLogin.ID,
	}
}

func GetLogContextFunctionForMessage(message *macos.Message) func(c zerolog.Context) zerolog.Context {
	return func(c zerolog.Context) zerolog.Context {
		return c.Str("message_guid", message.GUID)
	}
}

func (m *MessagesClient) QueueRemoteEventWrapper(evt bridgev2.RemoteEvent) {
	if m.DryRun {
		// m.UserLogin.Log.Info().Msgf("would send event: %s", evt.GetType())
		if asMessageEvent, ok := evt.(*simplevent.Message[macos.Message]); ok {
			m.UserLogin.Log.Info().Msgf("simpleEvent.Message type: %s", evt.GetType())

			context := context.TODO()
			portal := &bridgev2.Portal{
				Portal: &database.Portal{
					MXID: "foobar",
				},
			}

			if asMessageEvent.ConvertMessageFunc != nil {
				if true { //len(asMessageEvent.Data.CombinedComponents) > 1 || asMessageEvent.Data.Subject != "" {
					// m.UserLogin.Log.Info().Msgf("original:\n%s", asMessageEvent.Data)
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
					m.UserLogin.Log.Error().Msgf("error converting edit message: %v", err)
					return
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

	portalKey := m.PortalKeyFromMessage(message)
	m.UserLogin.Log.Info().Msgf("Queueing reaction sync for portal %s", portalKey.ID)
	m.QueueRemoteEventWrapper(&simplevent.ReactionSync{
		EventMeta: simplevent.EventMeta{
			Type:       bridgev2.RemoteEventReactionSync,
			LogContext: GetLogContextFunctionForMessage(message),
			PortalKey:  portalKey,
		},
		TargetMessage: networkid.MessageID(message.Tapback.TargetGUID),
		Reactions: &bridgev2.ReactionSyncData{
			Users: map[networkid.UserID]*bridgev2.ReactionSyncUser{
				networkid.UserID(message.HandleID): {
					HasAllReactions: true,
					Reactions:       reactions,
				},
			},
		},
	})
}

func (m *MessagesClient) HandleRetraction(message *macos.Message) {
	m.UserLogin.Log.Info().Msgf("Queueing message removal")
	m.QueueRemoteEventWrapper(&simplevent.MessageRemove{
		EventMeta: simplevent.EventMeta{
			Type:       bridgev2.RemoteEventMessageRemove,
			LogContext: GetLogContextFunctionForMessage(message),
			PortalKey:  m.PortalKeyFromMessage(message),
		},
		TargetMessage: networkid.MessageID(message.GUID),
		// OnlyForMe: true,
	})
}

func (m *MessagesClient) HandleEdit(message *macos.Message) {
	m.UserLogin.Log.Info().Msgf("Queueing message edit")
	m.QueueRemoteEventWrapper(&simplevent.Message[macos.Message]{
		EventMeta: simplevent.EventMeta{
			Sender: bridgev2.EventSender{
				Sender:   networkid.UserID(message.HandleID),
				IsFromMe: message.IsFromMe,
			},
			Type:         bridgev2.RemoteEventEdit,
			LogContext:   GetLogContextFunctionForMessage(message),
			PortalKey:    m.PortalKeyFromMessage(message),
			CreatePortal: true,
			Timestamp:    time.Now(),
		},
		TargetMessage:   networkid.MessageID(message.GUID),
		ID:              networkid.MessageID(message.GUID),
		Data:            *message,
		ConvertEditFunc: m.ConvertEditMessage,
	})
}

func (m *MessagesClient) HandleNormalMessage(message *macos.Message) {
	sender := networkid.UserID(message.HandleID)
	portalKey := m.PortalKeyFromMessage(message)
	m.UserLogin.Log.Info().Msgf("Queueing message")
	m.QueueRemoteEventWrapper(&simplevent.Message[macos.Message]{
		EventMeta: simplevent.EventMeta{
			Sender: bridgev2.EventSender{
				Sender:   sender,
				IsFromMe: message.IsFromMe,
			},
			Type: bridgev2.RemoteEventMessage,
			LogContext: func(c zerolog.Context) zerolog.Context {
				return c.
					Str("message_guid", message.GUID).
					Str("is_from_me", fmt.Sprintf("%t", message.IsFromMe)).
					Str("sender", string(sender)).
					Str("portalKey.ID", string(portalKey.ID)).
					Str("portalKey.Receiver", string(portalKey.Receiver))
			},
			PortalKey:    portalKey,
			CreatePortal: true,
			Timestamp:    time.Now(),
		},
		Data:               *message,
		ID:                 networkid.MessageID(message.GUID),
		ConvertMessageFunc: m.ConvertMessage,
	})
}

func (m *MessagesClient) GetGetUsersFunction(ctx context.Context, bridge *bridgev2.Bridge, portalKey networkid.PortalKey) func() ([]id.UserID, error) {
	return func() ([]id.UserID, error) {
		if m.DryRun {
			return []id.UserID{}, nil
		}
		userLogins, err := bridge.GetUserLoginsInPortal(ctx, portalKey)
		if err != nil {
			return nil, fmt.Errorf("getting user logins for portal %s: %v", portalKey, err)
		}
		users := make([]id.UserID, len(userLogins))
		for _, userLogin := range userLogins {
			users = append(users, userLogin.UserMXID)
		}
		return users, nil
	}
}

func (m *MessagesClient) ConvertEditMessage(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, existing []*database.Message, data macos.Message) (*bridgev2.ConvertedEdit, error) {
	currentParts, err := data.ConvertMessageToParts(ctx, intent, portal.MXID, m.GetGetUsersFunction(ctx, portal.Bridge, portal.PortalKey))
	if err != nil {
		return nil, fmt.Errorf("converting data message to parts: %w", err)
	}
	for i, part := range currentParts {
		part.ID = networkid.PartID(strconv.Itoa(i))
	}

	modifiedParts := []*bridgev2.ConvertedEditPart{}
	deletedParts := []*database.Message{}

	// EditedMessageParts is the source of truth for how many parts were originally in the message
	editedMessagePartsLength := len(data.EditedMessageParts)
	if editedMessagePartsLength == 0 {
		return nil, fmt.Errorf("edited message parts was empty")
	}
	if editedMessagePartsLength != len(existing) {
		return nil, fmt.Errorf("differing amount of parts in edited message (%d) vs existing message parts (%d): ", editedMessagePartsLength, len(existing))
	}

	// Strong assumption here that one cannot create new message parts when editing a message
	currentMessageIndex := 0
	for index := range editedMessagePartsLength {
		editedMessagePart := data.EditedMessageParts[index]
		existingMessagePart := existing[index]

		if editedMessagePart.Status == macos.EditedMessageStatusUnsent {
			/*
				// TODO: Consider modifying the existing message to indicate when it was deleted?
				who := "You"
				if !data.IsFromMe {
					who = "Sender"
				}
				suffix := "."
				if !data.EditedAt.IsZero() {
					if readableDateTimeDiff := dateTimeDiff(data.CreatedAt, data.EditedAt); readableDateTimeDiff != "" {
						suffix = fmt.Sprintf(" %s after sending%s", readableDateTimeDiff, suffix)
					}
				}
				convertedMessagePart.Content = &event.MessageEventContent{
					MsgType: event.MsgNotice,
					Body:    fmt.Sprintf("%s unsent this message part%s", who, suffix),
				}
				if !data.IsFromMe {
					username := "temp"
					server := "temp"
					name := "temp"
					convertedMessagePart.Content.Format = event.FormatHTML
					convertedMessagePart.Content.FormattedBody = fmt.Sprintf("%s unsent this message part%s", GetMentionText(username, server, name), suffix)
				}
			*/
			deletedParts = append(deletedParts, existingMessagePart)
			continue
		}

		modifiedParts = append(modifiedParts, currentParts[currentMessageIndex].ToEditPart(existingMessagePart))
		currentMessageIndex += 1
	}

	return &bridgev2.ConvertedEdit{
		ModifiedParts: modifiedParts,
		DeletedParts:  deletedParts,
	}, nil
}

func (m *MessagesClient) ConvertMessage(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, data macos.Message) (*bridgev2.ConvertedMessage, error) {
	var replyTo *networkid.MessageOptionalPartID
	if data.ReplyToGUID != "" {
		replyTo = &networkid.MessageOptionalPartID{
			MessageID: networkid.MessageID(data.ReplyToGUID),
		}
	}

	parts, err := data.ConvertMessageToParts(ctx, intent, portal.MXID, m.GetGetUsersFunction(ctx, portal.Bridge, portal.PortalKey))
	if err != nil {
		return nil, fmt.Errorf("converting data message to parts: %w", err)
	}
	for i, part := range parts {
		part.ID = networkid.PartID(strconv.Itoa(i))
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

func (m *MessagesClient) QueueMemberChatInfoChange(portalKey networkid.PortalKey, message *macos.Message, userID networkid.UserID, membership event.Membership) {
	m.UserLogin.Log.Info().Msgf("Queueing chat info change for portal %s and member %s to leave/ban (%t) or invite/join (%t)", portalKey.ID, userID, membership.IsLeaveOrBan(), membership.IsInviteOrJoin())
	m.QueueRemoteEventWrapper(&simplevent.ChatInfoChange{
		EventMeta: simplevent.EventMeta{
			Type:       bridgev2.RemoteEventChatInfoChange,
			LogContext: GetLogContextFunctionForMessage(message),
			PortalKey:  portalKey,
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
	portalKey := m.PortalKeyFromMessage(message)
	m.UserLogin.Log.Info().Msgf("Queueing chat info change for portal %s with leave/ban %t and invite/join %t", portalKey.ID, membership.IsLeaveOrBan(), membership.IsInviteOrJoin())
	m.QueueMemberChatInfoChange(portalKey, message, networkid.UserID(message.OtherID), membership)
}

func (m *MessagesClient) HandleName(message *macos.Message) {
	portalKey := m.PortalKeyFromMessage(message)
	m.UserLogin.Log.Info().Msgf("Queueing chat info change for group name for portal %s to %s", portalKey.ID, message.NewGroupTitle)
	m.QueueRemoteEventWrapper(&simplevent.ChatInfoChange{
		EventMeta: simplevent.EventMeta{
			Type:       bridgev2.RemoteEventChatInfoChange,
			LogContext: GetLogContextFunctionForMessage(message),
			PortalKey:  portalKey,
		},
		ChatInfoChange: &bridgev2.ChatInfoChange{
			ChatInfo: &bridgev2.ChatInfo{
				Name: &message.NewGroupTitle,
			},
		},
	})
}

func (m *MessagesClient) HandleAvatarOrMemberLeave(message *macos.Message) error {
	m.UserLogin.Log.Info().Msgf("Handling avatar or member leave with group action type %d", message.GroupActionType)
	portalKey := m.PortalKeyFromMessage(message)
	switch message.GroupActionType {
	case macos.GroupActionAddUser:
		// This happens when you leave a chat
		if message.HandleID == "" {
			message.HandleID = string(m.UserLogin.ID)
		}
		if message.ChatGUID == "" {
			return fmt.Errorf("[%d] no chat guid found for message leaving chat", message.DBRowID)
		}
		m.UserLogin.Log.Info().Msgf("Queueing event for user leaving portal %s", portalKey.ID)
		m.QueueMemberChatInfoChange(portalKey, message, networkid.UserID(message.HandleID), event.MembershipLeave)
	case macos.GroupActionSetAvatar:
		if len(message.Attachments) < 1 {
			return fmt.Errorf("[%d] no attachments found in update avatar message", message.DBRowID)
		}
		firstAttachment := message.Attachments[slices.Collect(maps.Keys(message.Attachments))[0]]
		firstAttachmentPathOnDisk := macos.ReplaceHomeDirectory(firstAttachment.PathOnDisk, m.UserHomeDir)
		m.UserLogin.Log.Info().Msgf("Queueing event for chat avatar change for portal %s", portalKey.ID)
		m.QueueRemoteEventWrapper(&simplevent.ChatInfoChange{
			EventMeta: simplevent.EventMeta{
				Type:       bridgev2.RemoteEventChatInfoChange,
				LogContext: GetLogContextFunctionForMessage(message),
				PortalKey:  portalKey,
			},
			ChatInfoChange: &bridgev2.ChatInfoChange{
				ChatInfo: &bridgev2.ChatInfo{
					Avatar: &bridgev2.Avatar{
						ID: networkid.AvatarID(fmt.Sprintf("%s-avatar", message.GUID)),
						Get: func(ctx context.Context) (result []byte, err error) {
							return os.ReadFile(firstAttachmentPathOnDisk)
						},
					},
				},
			},
		})
	case macos.GroupActionRemoveAvatar:
		m.UserLogin.Log.Info().Msgf("Queueing event for chat avatar removal for portal %s", portalKey)
		m.QueueRemoteEventWrapper(&simplevent.ChatInfoChange{
			EventMeta: simplevent.EventMeta{
				Type:       bridgev2.RemoteEventChatInfoChange,
				LogContext: GetLogContextFunctionForMessage(message),
				PortalKey:  portalKey,
			},
			ChatInfoChange: &bridgev2.ChatInfoChange{
				ChatInfo: &bridgev2.ChatInfo{
					Avatar: &bridgev2.Avatar{
						Remove: true,
					},
				},
			},
		})
	default:
		return fmt.Errorf("unrecognized message type combination (item_type: %d, group_action_type: %d)", message.ItemType, message.GroupActionType)
	}
	return nil
}

func (m *MessagesClient) HandleiMessage(message *macos.Message) error {
	m.UserLogin.Log.Info().Msgf("Handling message of type %d", message.ItemType)
	switch message.ItemType {
	case macos.ItemTypeMessage:
		m.HandleMessage(message)
	case macos.ItemTypeMember:
		m.HandleMember(message)
	case macos.ItemTypeName:
		m.HandleName(message)
	case macos.ItemTypeAvatar:
		return m.HandleAvatarOrMemberLeave(message)
	case macos.ItemTypeLocationSharing:
		m.UserLogin.Log.Warn().Msg("Skipping Location Sharing message")
	case macos.ItemTypeShareplay:
		m.UserLogin.Log.Warn().Msg("Skipping Shareplay message")
	default:
		return fmt.Errorf("skipped message [%s] of unknown type %d", message.GUID, message.ItemType)
	}
	return nil
}

func (m *MessagesClient) HandleiMessageReadReceipt(readReciept *macos.ReadReceipt) {
	m.UserLogin.Bridge.QueueRemoteEvent(m.UserLogin, &simplevent.Receipt{
		EventMeta: simplevent.EventMeta{
			Type:      bridgev2.RemoteEventReadReceipt,
			Timestamp: readReciept.ReadAt,
			PortalKey: networkid.PortalKey{
				ID:       macos.MakeMessagesPortalID(m.UserLogin.ID, readReciept.ChatGUID),
				Receiver: m.UserLogin.ID,
			},
			Sender: bridgev2.EventSender{
				IsFromMe:    readReciept.IsFromMe,
				SenderLogin: networkid.UserLoginID(readReciept.SenderGUID),
			},
		},
		LastTarget: networkid.MessageID(readReciept.ReadUpTo),
	})
}
