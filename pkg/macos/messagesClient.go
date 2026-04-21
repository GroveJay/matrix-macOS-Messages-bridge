package macos

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog"
	"howett.net/plist"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
)

const (
	MACOS_16_MESSAGES_COLUMNS    = 95
	MACOS_14_MESSAGES_COLUMNS    = 88
	MACOS_16_ATTACHMENTS_COLUMNS = 27
	MACOS_14_ATTACHMENTS_COLUMNS = 24
	ADDITIONAL_MESSAGES_COLUMNS  = 4
)

type ReadReceipt struct {
	ChatGUID       string
	ChatHandlesIDs string
	ReadUpTo       string
	ReadAt         time.Time
	IsFromMe       bool
	SenderGUID     string
}

func ParseIdentifier(identifier string) Identifier {
	if len(identifier) == 0 {
		return Identifier{}
	}
	parts := strings.Split(identifier, ";")
	return Identifier{
		Service: parts[0],
		IsGroup: parts[1] == "+",
		LocalID: parts[2],
	}
}

func (id Identifier) String() string {
	if len(id.LocalID) == 0 {
		return ""
	}
	typeChar := '-'
	if id.IsGroup {
		typeChar = '+'
	}
	return fmt.Sprintf("%s;%c;%s", id.Service, typeChar, id.LocalID)
}

type MacOSMessagesClient struct {
	log                    *zerolog.Logger
	chatDB                 *sql.DB
	chatDBPath             string
	userHomeDir            string
	groupMemberQuery       *sql.Stmt
	chatQuery              *sql.Stmt
	chatHandlesIDsQuery    *sql.Stmt
	groupActionQuery       *sql.Stmt
	maxMessagesTimeQuery   *sql.Stmt
	newMessagesQuery       *sql.Stmt
	messagesNewerThanQuery *sql.Stmt
	messagesBetweenQuery   *sql.Stmt
	messagesByGUIDQuery    *sql.Stmt
	newReceiptsQuery       *sql.Stmt
	attachmentsQuery       *sql.Stmt
}

func GetMessagesClient(userName string, logger *zerolog.Logger) (*MacOSMessagesClient, error) {
	client := &MacOSMessagesClient{
		log: logger,
	}
	var err error
	if client.chatDB, client.chatDBPath, err = openChatDB(); err != nil {
		return nil, fmt.Errorf("failed to open chat db: %w", err)
	}
	if client.userHomeDir, err = os.UserHomeDir(); err != nil {
		return nil, fmt.Errorf("failed to get user home dir: %w", err)
	}

	if client.groupMemberQuery, err = client.chatDB.Prepare(GroupMemberQuery); err != nil {
		return nil, fmt.Errorf("[%s] failed to prepare group query: %w", client.chatDBPath, err)
	}
	if client.chatQuery, err = client.chatDB.Prepare(ChatQuery); err != nil {
		return nil, fmt.Errorf("failed to prepare chat query: %w", err)
	}
	if client.groupActionQuery, err = client.chatDB.Prepare(GroupActionQuery); err != nil {
		return nil, fmt.Errorf("failed to prepare group action query: %w", err)
	}
	if client.maxMessagesTimeQuery, err = client.chatDB.Prepare(MaxMessagesTimeQuery); err != nil {
		return nil, fmt.Errorf("failed to prepare max messages time query: %w", err)
	}
	if client.newMessagesQuery, err = client.chatDB.Prepare(NewMessagesQuery); err != nil {
		return nil, fmt.Errorf("failed to prepare new messages query: %w", err)
	}
	if client.messagesNewerThanQuery, err = client.chatDB.Prepare(MessagesNewerThanQuery); err != nil {
		return nil, fmt.Errorf("failed to prepare newer than messages query: %w", err)
	}
	if client.messagesBetweenQuery, err = client.chatDB.Prepare(MessagesBetweenQuery); err != nil {
		return nil, fmt.Errorf("failed to prepare messages between query: %w", err)
	}
	if client.messagesByGUIDQuery, err = client.chatDB.Prepare(MessagesByGUID); err != nil {
		return nil, fmt.Errorf("failed to prepare messages by GUID: %w", err)
	}
	if client.newReceiptsQuery, err = client.chatDB.Prepare(NewRecieptsQuery); err != nil {
		return nil, fmt.Errorf("failed to prepare new reciepts query: %w", err)
	}
	if client.attachmentsQuery, err = client.chatDB.Prepare(AttachmentsQuery); err != nil {
		return nil, fmt.Errorf("failed to prepare attachments query: %w", err)
	}
	return client, nil
}

func (c MacOSMessagesClient) ValidateConnection() error {
	_, _, err := RunOsascript(CheckMessagesRunning)
	if err != nil {
		return fmt.Errorf("failed Messages running check: %v", err)
	}
	return nil
}

func (c MacOSMessagesClient) GetChatDBPath() string {
	return c.chatDBPath
}

func (c MacOSMessagesClient) GetChatDBWALPath() string {
	return fmt.Sprintf("%s-wal", c.chatDBPath)
}

func (c MacOSMessagesClient) GetChatMemberMap(chatGUID string, selfUserID networkid.UserID) (map[networkid.UserID]bridgev2.ChatMember, error) {
	if members, err := c.getGroupMembers(chatGUID); err != nil {
		return nil, err
	} else {
		membersMap := make(map[networkid.UserID]bridgev2.ChatMember)
		for _, member := range members {
			membersMap[networkid.UserID(member)] = bridgev2.ChatMember{
				Membership: event.MembershipJoin,
				UserInfo: &bridgev2.UserInfo{
					Identifiers: []string{},
				},
			}
		}
		if selfMember, ok := membersMap[selfUserID]; !ok {
			membersMap[selfUserID] = bridgev2.ChatMember{
				Membership: event.MembershipJoin,
				EventSender: bridgev2.EventSender{
					IsFromMe: true,
				},
				UserInfo: &bridgev2.UserInfo{
					Identifiers: []string{},
				},
			}
		} else {
			selfMember.EventSender.IsFromMe = true
		}

		return membersMap, nil
	}
}

func (c *MacOSMessagesClient) GetChatDetails(chatID networkid.PortalID) (*string, *string, *bridgev2.Avatar, error) {
	chatGUID, err := c.GetChatGUIDFromPortalID(chatID)
	if err != nil {
		return nil, nil, nil, err
	}
	chatRow := c.chatQuery.QueryRow(chatGUID)
	var name string
	if err := chatRow.Scan(&name); err != nil {
		return &chatGUID, nil, nil, err
	}

	avatarRow := c.groupActionQuery.QueryRow(ItemTypeAvatar, GroupActionSetAvatar, chatGUID)
	var fileName string
	var mimeType string
	var path string

	if err := avatarRow.Scan(path, mimeType, fileName); err != nil {
		if err != sql.ErrNoRows {
			return &chatGUID, &name, nil, err
		}
		return &chatGUID, &name, nil, nil
	}
	path = ReplaceHomeDirectory(path, c.userHomeDir)
	avatar := &bridgev2.Avatar{
		ID: networkid.AvatarID(fmt.Sprintf("%s-%s", chatGUID, fileName)),
		Get: func(ctx context.Context) ([]byte, error) {
			file, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			return AddMessagesIconToAvatarImage(file)
		},
	}
	return &chatGUID, &name, avatar, nil
}

func (c *MacOSMessagesClient) GetMaxMessagesTime() (*int64, error) {
	var maxMessagesTimeSQL sql.NullInt64
	rows, err := c.maxMessagesTimeQuery.Query()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch maximum message time: %w", err)
	}
	if !rows.Next() {
		err = rows.Err()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("no result getting maximum message time")
	}
	err = rows.Scan(&maxMessagesTimeSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to scan maximum message time: %w", err)
	} else if !maxMessagesTimeSQL.Valid {
		return nil, fmt.Errorf("invalid maximum message time")
	}
	return &maxMessagesTimeSQL.Int64, nil
}

func (c *MacOSMessagesClient) GetMessagesAboveRowID(rowID int) ([]*DBMessage, error) {
	res, err := c.newMessagesQuery.Query(rowID)
	if err != nil {
		return nil, fmt.Errorf("error querying messages after rowid: %w", err)
	}
	return c.parseMessages(res)
}

func (c *MacOSMessagesClient) GetMessagesNewerThan(t int64) ([]*DBMessage, error) {
	res, err := c.messagesNewerThanQuery.Query(t)
	if err != nil {
		return nil, fmt.Errorf("error querying messages after time %d: %w", t, err)
	}
	return c.parseMessages(res)
}

func (c *MacOSMessagesClient) GetMessagesBetween(minRowID int, maxRowID int) ([]*DBMessage, error) {
	res, err := c.messagesBetweenQuery.Query(minRowID, maxRowID)
	if err != nil {
		return nil, fmt.Errorf("error querying messages between rowids %d and %d: %w", minRowID, maxRowID, err)
	}
	return c.parseMessages(res)
}

func (c *MacOSMessagesClient) GetMessageByGUID(guid string) (*DBMessage, error) {
	res, err := c.messagesByGUIDQuery.Query(guid)
	if err != nil {
		return nil, fmt.Errorf("error querying message by GUID %s: %w", guid, err)
	}
	results, err := c.parseMessages(res)
	if err != nil {
		return nil, fmt.Errorf("error parsing messages for GUID %s: %w", guid, err)
	}
	if len(results) != 1 {
		return nil, fmt.Errorf("more than one message result for GUID %s", guid)
	}
	return results[0], nil
}

func ReadRecieptScan(res *sql.Rows) (*ReadReceipt, error) {
	var readReceipt ReadReceipt
	var chatGUID, chatHandlesIDs string
	var messageIsFromMe bool
	var readAtAppleEpoch int64
	err := res.Scan(&readReceipt.ChatGUID, &chatHandlesIDs, &readReceipt.ReadUpTo, &messageIsFromMe, &readAtAppleEpoch)
	if err == nil {
		if messageIsFromMe {
			// For messages from me, the receipt is not from me, and vice versa.
			readReceipt.IsFromMe = false
			if ParseIdentifier(readReceipt.ChatGUID).IsGroup {
				// We don't get read receipts from other users in groups,
				// so skip our own messages.
				return nil, nil
			} else {
				// The read receipt is on our own message and it's a private chat,
				// which means the read receipt is from the private chat recipient.
				readReceipt.SenderGUID = chatGUID
			}
		} else {
			readReceipt.IsFromMe = true
		}
		readReceipt.ReadAt = time.Unix(AppleEpochUnix, readAtAppleEpoch)
		readReceipt.ChatHandlesIDs = SortChatHandlesIDs(chatHandlesIDs)
	}

	return &readReceipt, err
}

func (c *MacOSMessagesClient) GetReadReceiptsSince(minDate time.Time) ([]*ReadReceipt, time.Time, error) {
	origMinDate := minDate.UnixNano() - AppleEpochUnixNano
	res, err := c.newReceiptsQuery.Query(origMinDate)
	if err != nil {
		return nil, minDate, fmt.Errorf("error querying read receipts after date: %w", err)
	}
	var receipts []*ReadReceipt
	for res.Next() {
		if readReceipt, err := ReadRecieptScan(res); err != nil {
			return receipts, minDate, fmt.Errorf("error scanning row: %w", err)
		} else if readReceipt == nil {
			continue
		} else {
			if readReceipt.ReadAt.After(minDate) {
				minDate = readReceipt.ReadAt
			}
			receipts = append(receipts, readReceipt)
		}
	}
	return receipts, minDate, nil
}

func openChatDB() (*sql.DB, string, error) {
	path, err := os.UserHomeDir()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get home directory: %w", err)
	}
	path = filepath.Join(path, "Library", "Messages", "chat.db")
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro&_query_only=true", path))
	// db.SetMaxOpenConns(1)
	return db, path, err
}

func (c *MacOSMessagesClient) getGroupMembers(chatID string) (users []networkid.UserID, err error) {
	res, err := c.groupMemberQuery.Query(chatID)
	if err != nil {
		return nil, fmt.Errorf("error querying group members: %w", err)
	}
	for res.Next() {
		var user, country string
		err = res.Scan(&user, &country)
		if err != nil {
			return users, fmt.Errorf("error scanning row: %w", err)
		} else if len(user) == 0 {
			continue
		}
		if strings.Contains(user, "@") {
			users = append(users, networkid.UserID(user))
			continue
		}
		if strings.Contains(user, ":") {
			users = append(users, networkid.UserID(user))
			continue
		}
		if userID, err := ParseFormatPhoneNumber(user, country); err != nil {
			return users, fmt.Errorf("error parsing user (%s): %w", user, err)
		} else {
			users = append(users, *userID)
		}
	}
	return users, nil
}

func OS16MessagesScan(res *sql.Rows) (*DBMessage, error) {
	var message DBMessage
	var dummyText sql.NullString
	var dummyInt sql.NullInt64
	var testInt sql.NullString

	var messageText sql.NullString
	var messageSubject sql.NullString
	var newGroupTitle sql.NullString
	var threadOriginatorGUID sql.NullString
	var threadOriginatorPart sql.NullString
	var balloonBundleID sql.NullString
	var handleID sql.NullString
	var otherID sql.NullString
	var tapbackTargetGUID sql.NullString
	var tapbackEmoji sql.NullString
	var chatGUID sql.NullString
	var chatHandlesIDs sql.NullString

	// TODO add expressive_send_style_id here
	err := res.Scan(
		&message.RowID, &message.GUID, &messageText, &dummyInt, &dummyText, &testInt, &messageSubject, &dummyText, &message.AttributedBody, &dummyInt,
		&dummyInt, &dummyText, &dummyText, &dummyText, &dummyInt, &message.Date, &message.DateRead, &dummyInt, &message.IsDelivered, &dummyInt,
		&message.IsEmote, &message.IsFromMe, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &message.IsSent, &dummyInt,
		&dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyText, &dummyInt, &dummyInt, &message.IsAudioMessage, &dummyInt,
		&dummyInt, &message.ItemType, &dummyInt, &newGroupTitle, &message.GroupActionType, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt,
		&dummyInt, &tapbackTargetGUID, &message.TapbackType, &balloonBundleID, &message.PayloadData, &dummyText, &dummyInt, &dummyInt, &dummyInt, &message.MessageSummaryInfo,
		&dummyInt, &dummyText, &dummyText, &dummyText, &dummyInt, &dummyText, &dummyText, &dummyInt, &dummyText, &dummyInt,
		&dummyInt, &dummyInt, &threadOriginatorGUID, &threadOriginatorPart, &dummyText, &dummyInt, &dummyInt, &dummyText, &message.DateRetracted, &message.DateEdited,
		&dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyText, &dummyInt, &dummyText, &tapbackEmoji, &dummyInt,
		&dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt,
		&chatGUID, &chatHandlesIDs, &handleID, &otherID,
	)
	if err == nil {
		messageStringFields := map[*string]sql.NullString{
			&message.Text:                 messageText,
			&message.NewGroupTitle:        newGroupTitle,
			&message.Subject:              messageSubject,
			&message.ReplyToGUID:          threadOriginatorGUID,
			&message.BalloonBundleID:      balloonBundleID,
			&message.HandleID:             handleID,
			&message.OtherID:              otherID,
			&message.ChatGUID:             chatGUID,
			&message.ChatHandlesIDs:       chatHandlesIDs,
			&message.TapbackTargetGUID:    tapbackTargetGUID,
			&message.TapbackEmoji:         tapbackEmoji,
			&message.ThreadOriginatorPart: threadOriginatorPart,
		}
		for field, value := range messageStringFields {
			if value.Valid {
				*field = value.String
			}
		}
	}
	return &message, err
}

func OS14MessagesScan(res *sql.Rows) (*DBMessage, error) {
	var message DBMessage
	var dummyText sql.NullString
	var dummyInt sql.NullInt64

	var messageText sql.NullString
	var messageSubject sql.NullString
	var newGroupTitle sql.NullString
	var threadOriginatorGUID sql.NullString
	var threadOriginatorPart sql.NullString
	var tapbackTargetGUID sql.NullString
	var balloonBundleID sql.NullString
	var handleID sql.NullString
	var otherID sql.NullString
	var chatGUID sql.NullString
	var chatHandlesIDs sql.NullString

	err := res.Scan(
		&message.RowID, &message.GUID, &messageText, &dummyInt, &dummyText, &dummyInt, &messageSubject, &dummyText, &message.AttributedBody, &dummyInt,
		&dummyInt, &dummyText, &dummyText, &dummyText, &dummyInt, &message.Date, &message.DateRead, &dummyInt, &message.IsDelivered, &dummyInt,
		&message.IsEmote, &message.IsFromMe, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &message.IsSent, &dummyInt,
		&dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyText, &dummyInt, &dummyInt, &message.IsAudioMessage, &dummyInt,
		&dummyInt, &message.ItemType, &dummyInt, &newGroupTitle, &message.GroupActionType, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt,
		&dummyInt, &tapbackTargetGUID, &message.TapbackType, &balloonBundleID, &message.PayloadData, &dummyText, &dummyInt, &dummyInt, &dummyInt, &message.MessageSummaryInfo,
		&dummyInt, &dummyText, &dummyText, &dummyText, &dummyInt, &dummyText, &dummyText, &dummyInt, &dummyText, &dummyInt,
		&dummyInt, &dummyInt, &threadOriginatorGUID, &threadOriginatorPart, &dummyText, &dummyInt, &dummyInt, &dummyText, &message.DateRetracted, &message.DateEdited,
		&dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyText, &dummyInt, &dummyText,
		&chatGUID, &chatHandlesIDs, &handleID, &otherID,
	)
	if err == nil {
		messageStringFields := map[*string]sql.NullString{
			&message.Text:                 messageText,
			&message.NewGroupTitle:        newGroupTitle,
			&message.Subject:              messageSubject,
			&message.ReplyToGUID:          threadOriginatorGUID,
			&message.BalloonBundleID:      balloonBundleID,
			&message.HandleID:             handleID,
			&message.OtherID:              otherID,
			&message.ChatGUID:             chatGUID,
			&message.ChatHandlesIDs:       chatHandlesIDs,
			&message.TapbackTargetGUID:    tapbackTargetGUID,
			&message.ThreadOriginatorPart: threadOriginatorPart,
		}
		for field, value := range messageStringFields {
			if value.Valid {
				*field = value.String
			}
		}
	}

	return &message, err
}

func GetMessagesScanFunctionForColumns(res *sql.Rows) (func(res *sql.Rows) (*DBMessage, error), error) {
	columns, err := res.Columns()
	if err != nil {
		err = fmt.Errorf("getting columns for messages query: %w", err)
		return nil, err
	}
	// TODO: Actually check the columns are exactly as expected
	columnCount := len(columns)
	if columnCount == (MACOS_16_MESSAGES_COLUMNS + ADDITIONAL_MESSAGES_COLUMNS) {
		return OS16MessagesScan, nil
	} else if columnCount == (MACOS_14_MESSAGES_COLUMNS + ADDITIONAL_MESSAGES_COLUMNS) {
		return OS14MessagesScan, nil
	} else {
		return nil, fmt.Errorf("unrecognized column count (%d) in Message 'message' database", columnCount)
	}
}

// SELECT guid, COALESCE(filename, ''), COALESCE(mime_type, ''), transfer_name, is_sticker, sticker_user_info, COALESCE(emoji_image_short_description, '')
// err = ares.Scan(&attachment.GUID, &attachment.PathOnDisk, &attachment.MimeType, &attachment.FileName, &attachment.IsSticker, &stickerUserInfo, &attachment.EmojiImageShortDescription)
/*
20|sr_ck_sync_state|INTEGER|0|0|0
21|sr_ck_server_change_token_blob|BLOB|0||0
22|sr_ck_record_id|TEXT|0||0
23|is_commsafety_sensitive|INTEGER|0|0|0
24|emoji_image_content_identifier|TEXT|0|NULL|0
25|emoji_image_short_description|TEXT|0|NULL|0
26|preview_generation_state|INTEGER|0|0|0
*/
func OS16AttachmentScan(attachmentRows *sql.Rows) (Attachment, []byte, error) {
	var attachment Attachment
	var stickerUserInfo []byte

	var dummyText sql.NullString
	var dummyInt sql.NullInt64
	var dummyBlob []byte

	var filename sql.NullString
	var mimeType sql.NullString
	var emojiImageShortDescription sql.NullString

	err := attachmentRows.Scan(
		&dummyInt, &attachment.GUID, &dummyInt, &dummyInt, &filename, &dummyText, &mimeType, &dummyInt, &dummyInt, &dummyBlob,
		&attachment.FileName, &dummyInt, &attachment.IsSticker, &stickerUserInfo, &dummyBlob, &dummyInt, &dummyInt, &dummyBlob, &dummyText, &dummyText,
		&dummyInt, &dummyBlob, &dummyText, &dummyInt, &dummyText, &emojiImageShortDescription, &dummyInt,
	)
	if err != nil {
		return attachment, stickerUserInfo, err
	}

	attachmentStringFields := map[*string]sql.NullString{
		&attachment.PathOnDisk:                 filename,
		&attachment.MimeType:                   mimeType,
		&attachment.EmojiImageShortDescription: emojiImageShortDescription,
	}
	for field, value := range attachmentStringFields {
		if value.Valid {
			*field = value.String
		}
	}

	return attachment, stickerUserInfo, nil
}

func OS14AttachmentScan(attachmentRows *sql.Rows) (Attachment, []byte, error) {
	var attachment Attachment
	var stickerUserInfo []byte

	var dummyText sql.NullString
	var dummyInt sql.NullInt64
	var dummyBlob []byte

	var filename sql.NullString
	var mimeType sql.NullString

	err := attachmentRows.Scan(
		&dummyInt, &attachment.GUID, &dummyInt, &dummyInt, &filename, &dummyText, &mimeType, &dummyInt, &dummyInt, &dummyBlob,
		&attachment.FileName, &dummyInt, &attachment.IsSticker, &stickerUserInfo, &dummyBlob, &dummyInt, &dummyInt, &dummyBlob, &dummyText, &dummyText,
		&dummyInt, &dummyBlob, &dummyText, &dummyInt,
	)
	if err != nil {
		return attachment, stickerUserInfo, err
	}

	attachmentStringFields := map[*string]sql.NullString{
		&attachment.PathOnDisk: filename,
		&attachment.MimeType:   mimeType,
	}
	for field, value := range attachmentStringFields {
		if value.Valid {
			*field = value.String
		}
	}

	return attachment, stickerUserInfo, nil
}

func GetAttachmentsScanFunctionForColumns(attachmentRows *sql.Rows) (func(attachmentRows *sql.Rows) (attachment Attachment, stickerUserInfo []byte, err error), error) {
	columns, err := attachmentRows.Columns()
	if err != nil {
		err = fmt.Errorf("getting columns for attachments query: %w", err)
		return nil, err
	}
	// TODO: Check the columns are exactly as expected
	columnCount := len(columns)
	if columnCount == MACOS_16_ATTACHMENTS_COLUMNS {
		return OS16AttachmentScan, nil
	} else if columnCount == MACOS_14_ATTACHMENTS_COLUMNS {
		return OS14AttachmentScan, nil
	} else {
		return nil, fmt.Errorf("unrecognized column count (%d) in Message 'attachment' database", columnCount)
	}
}

func (c *MacOSMessagesClient) getAttachments(rowID int) (map[string]*Attachment, error) {
	attachments := map[string]*Attachment{}

	attachmentRows, err := c.attachmentsQuery.Query(rowID)
	if err != nil {
		return nil, fmt.Errorf("querying attachments for %d: %w", rowID, err)
	}
	var attachmentsScanFunction func(*sql.Rows) (Attachment, []byte, error)
	attachmentsScanFunction, err = GetAttachmentsScanFunctionForColumns(attachmentRows)
	if err != nil {
		err = fmt.Errorf("getting Attachments scan function: %w", err)
		return nil, err
	}
	for attachmentRows.Next() {
		attachment, stickerUserInfo, err := attachmentsScanFunction(attachmentRows)
		if err != nil {
			return nil, fmt.Errorf("error scanning attachment row for %d: %w", rowID, err)
		}
		attachment.PathOnDisk = ReplaceHomeDirectory(attachment.PathOnDisk, c.userHomeDir)
		if len(stickerUserInfo) > 0 {
			plistDictionary := make(map[string]any, 0)
			if err := plist.NewDecoder(bytes.NewReader(stickerUserInfo)).Decode(plistDictionary); err != nil {
				return nil, fmt.Errorf("decoding plist to plistDictionary: %w", err)
			}
			pid, err := GetValueAsTypeFromMapKey[string](plistDictionary, "pid")
			if err != nil {
				return nil, fmt.Errorf("finding pid key in plistDictionary: %w", err)
			}
			attachment.StickerSource = StickerSource(*pid)
		}
		// TODO: add attribution_info parsing, meh
		attachments[attachment.GUID] = &attachment
	}

	return attachments, nil
}

func (c *MacOSMessagesClient) parseMessages(res *sql.Rows) ([]*DBMessage, error) {
	messagesScanFunction, err := GetMessagesScanFunctionForColumns(res)
	if err != nil {
		return nil, fmt.Errorf("getting row scan function: %w", err)
	}
	// TODO: allocate this ahead of time to avoid append
	dbMessages := []*DBMessage{}
	for res.Next() {
		dbMessage, err := messagesScanFunction(res)
		if err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}

		dbMessage.Attachments, err = c.getAttachments(dbMessage.RowID)
		if err != nil {
			return nil, fmt.Errorf("getting attachments: %w", err)
		}

		dbMessages = append(dbMessages, dbMessage)
	}
	return dbMessages, nil
}

func (c *MacOSMessagesClient) SendMessage(portalID networkid.PortalID, body string) error {
	if body == "" {
		return fmt.Errorf("message content body was empty")
	}
	chatGUID, err := c.GetChatGUIDFromPortalID(portalID)
	if err != nil {
		return fmt.Errorf("error getting chat GUID from Portal ID %s: %v", portalID, err)
	}
	_, stderr, err := RunOsascript(SendMessageToChatGUID, chatGUID, body)
	if err != nil {
		return fmt.Errorf("error sending message of length %d to chatGUID %s: %v", len(body), chatGUID, err)
	}
	if stderr != "" {
		return fmt.Errorf("stderr was not empty sending message to chatGUID %s of length %d: %s", chatGUID, len(body), stderr)
	}
	return nil
}

func (c *MacOSMessagesClient) GetChatGUIDFromPortalID(portalID networkid.PortalID) (string, error) {
	portalChatHandlesIDsString := ChatHandlesIDsFromPortalID(portalID)
	if portalChatHandlesIDsString == "" {
		return "", fmt.Errorf("empty chat handles ids from incoming message Portal ID: %s", portalID)
	}
	stdout, stderr, err := RunOsascript(GetChatGUIDFromHandlesIDs, portalChatHandlesIDsString)
	if err != nil || len(stdout) == 0 || len(stderr) != 0 {
		return "", fmt.Errorf("error getting chat GUID from chat handles ids %s: %v\nstdout:\n%s\nstderr:\n%s", portalChatHandlesIDsString, err, stdout, stderr)
	}
	return strings.TrimSuffix(stdout, "\n"), nil
}
