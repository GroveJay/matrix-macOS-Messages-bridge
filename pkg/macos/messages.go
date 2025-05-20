package macos

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog"
	"howett.net/plist"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
)

type MacOSMessagesClient struct {
	log                    *zerolog.Logger
	chatDB                 *sql.DB
	chatDBPath             string
	groupMemberQuery       *sql.Stmt
	chatQuery              *sql.Stmt
	groupActionQuery       *sql.Stmt
	maxMessagesRowQuery    *sql.Stmt
	maxMessagesTimeQuery   *sql.Stmt
	newMessagesQuery       *sql.Stmt
	messagesNewerThanQuery *sql.Stmt
	messagesBetweenQuery   *sql.Stmt
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

	if client.groupMemberQuery, err = client.chatDB.Prepare(GroupMemberQuery); err != nil {
		return nil, fmt.Errorf("[%s] failed to prepare group query: %w", client.chatDBPath, err)
	}
	if client.chatQuery, err = client.chatDB.Prepare(ChatQuery); err != nil {
		return nil, fmt.Errorf("failed to prepare chat query: %w", err)
	}
	if client.groupActionQuery, err = client.chatDB.Prepare(GroupActionQuery); err != nil {
		return nil, fmt.Errorf("failed to prepare group action query: %w", err)
	}
	if client.maxMessagesRowQuery, err = client.chatDB.Prepare(MaxMessagesRowQuery); err != nil {
		return nil, fmt.Errorf("failed to prepare max messages row query: %w", err)
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
	if client.newReceiptsQuery, err = client.chatDB.Prepare(NewRecieptsQuery); err != nil {
		return nil, fmt.Errorf("failed to prepare new reciepts query: %w", err)
	}
	if client.attachmentsQuery, err = client.chatDB.Prepare(AttachmentsQuery); err != nil {
		return nil, fmt.Errorf("failed to prepare attachments query: %w", err)
	}
	return client, nil
}

func (c MacOSMessagesClient) ValidateConnection() error {

	return nil
}

func (c MacOSMessagesClient) GetChatDBPath() string {
	return c.chatDBPath
}

func (c MacOSMessagesClient) GetChatMemberMap(chatID networkid.PortalID, selfUserID networkid.UserID) (map[networkid.UserID]bridgev2.ChatMember, error) {
	if members, err := c.getGroupMembers(string(chatID)); err != nil {
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
		if _, ok := membersMap[selfUserID]; !ok {
			membersMap[selfUserID] = bridgev2.ChatMember{
				Membership: event.MembershipJoin,
				EventSender: bridgev2.EventSender{
					IsFromMe: true,
				},
			}
		}

		return membersMap, nil
	}
}

func (c *MacOSMessagesClient) GetChatDetails(chatID networkid.PortalID) (*string, *bridgev2.Avatar, error) {
	chatGUID := ChatGUIDFromPortalID(chatID)
	chatRow := c.chatQuery.QueryRow(chatGUID)
	var name string
	if err := chatRow.Scan(&name); err != nil {
		return nil, nil, err
	} else if name == "" {
		name = chatGUID
	}
	avatarRow := c.groupActionQuery.QueryRow(ItemTypeAvatar, GroupActionSetAvatar, chatGUID)
	var fileName string
	var mimeType string
	var path string

	if err := avatarRow.Scan(path, mimeType, fileName); err != nil {
		if err != sql.ErrNoRows {
			return &name, nil, err
		}
		return &name, nil, nil
	}
	path, err := ReplaceHomeDirectory(path)
	if err != nil {
		return &name, nil, err
	}
	avatar := &bridgev2.Avatar{
		ID: networkid.AvatarID(fmt.Sprintf("%s-%s", chatGUID, fileName)),
		Get: func(ctx context.Context) ([]byte, error) {
			return os.ReadFile(path)
		},
	}
	return &name, avatar, nil
}

func (c *MacOSMessagesClient) GetAllChatIDsNames() (map[string]string, error) {
	stdout, stderr, err := RunOsascript(GetChatIDsNames)
	if err != nil || len(stdout) == 0 || len(stderr) != 0 {
		return nil, fmt.Errorf("%w:\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	chatMap := make(map[string]string)
	for _, line := range strings.Split(stdout, "\n") {
		if len(line) == 0 {
			continue
		}
		line_parts := strings.Split(line, "|")
		id := line_parts[0]
		if len(line_parts) > 1 {
			chatMap[id] = line_parts[1]
		} else {
			chatMap[id] = ""
		}
	}
	return chatMap, nil
}

func (c *MacOSMessagesClient) GetMaxMessagesRow() (*int, error) {
	var lastRowIDSQL sql.NullInt32
	err := c.maxMessagesRowQuery.QueryRow("SELECT MAX(ROWID) FROM message").Scan(&lastRowIDSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch last row ID: %w", err)
	} else if !lastRowIDSQL.Valid {
		return nil, fmt.Errorf("invalid last row ID")
	}
	lastRowInt := int(lastRowIDSQL.Int32)
	return &lastRowInt, nil
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

func (c *MacOSMessagesClient) GetMessagesAboveRowID(rowID int) ([]*Message, error) {
	res, err := c.newMessagesQuery.Query(rowID)
	if err != nil {
		return nil, fmt.Errorf("error querying messages after rowid: %w", err)
	}
	return c.parseMessages(res)
}

func (c *MacOSMessagesClient) GetMessagesNewerThan(t int64) ([]*Message, error) {
	res, err := c.messagesNewerThanQuery.Query(t)
	if err != nil {
		return nil, fmt.Errorf("error querying messages after time %d: %w", t, err)
	}
	return c.parseMessages(res)
}

func (c *MacOSMessagesClient) GetMessagesBetween(minRowID int, maxRowID int) ([]*Message, error) {
	res, err := c.messagesBetweenQuery.Query(minRowID, maxRowID)
	if err != nil {
		return nil, fmt.Errorf("error querying messages between rowids %d and %d: %w", minRowID, maxRowID, err)
	}
	return c.parseMessages(res)
}

func (c *MacOSMessagesClient) GetReadReceiptsSince(minDate time.Time) ([]*ReadReceipt, time.Time, error) {
	origMinDate := minDate.UnixNano() - AppleEpochUnixNano
	res, err := c.newReceiptsQuery.Query(origMinDate)
	if err != nil {
		return nil, minDate, fmt.Errorf("error querying read receipts after date: %w", err)
	}
	var receipts []*ReadReceipt
	for res.Next() {
		var chatGUID, messageGUID string
		var messageIsFromMe bool
		var readAtAppleEpoch int64
		err = res.Scan(&chatGUID, &messageGUID, &messageIsFromMe, &readAtAppleEpoch)
		if err != nil {
			return receipts, minDate, fmt.Errorf("error scanning row: %w", err)
		}
		readAt := time.Unix(AppleEpochUnix, readAtAppleEpoch)
		if readAtAppleEpoch > origMinDate {
			minDate = readAt
		}

		receipt := &ReadReceipt{
			ChatGUID: chatGUID,
			ReadUpTo: messageGUID,
			ReadAt:   readAt,
		}
		if messageIsFromMe {
			// For messages from me, the receipt is not from me, and vice versa.
			receipt.IsFromMe = false
			if ParseIdentifier(chatGUID).IsGroup {
				// We don't get read receipts from other users in groups,
				// so skip our own messages.
				continue
			} else {
				// The read receipt is on our own message and it's a private chat,
				// which means the read receipt is from the private chat recipient.
				receipt.SenderGUID = chatGUID
			}
		} else {
			receipt.IsFromMe = true
		}
		receipts = append(receipts, receipt)
	}
	return receipts, minDate, nil
}

func openChatDB() (*sql.DB, string, error) {
	path, err := os.UserHomeDir()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get home directory: %w", err)
	}
	path = filepath.Join(path, "Library", "Messages", "chat.db")
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro", path))
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
		if userID, err := ParseFormatPhoneNumber(user, country); err != nil {
			return users, fmt.Errorf("error parsing user (%s): %w", user, err)
		} else {
			users = append(users, *userID)
		}

	}
	return users, nil
}

func OS16MessagesScan(res *sql.Rows, message *Message, attributedBody *[]byte, messageSummaryInfo *[]byte, tapback *Tapback, threadOriginatorPartString *string) error {
	var dummyText sql.NullString
	var dummyInt sql.NullInt64
	var dummyBlob []byte

	var messageText sql.NullString
	var messageSubject sql.NullString
	var newGroupTitle sql.NullString
	var threadOriginatorGUID sql.NullString
	var threadOriginatorPart sql.NullString
	var balloonBundleID sql.NullString
	var senderLocalID sql.NullString
	var senderService sql.NullString
	var targetLocalID sql.NullString
	var targetService sql.NullString
	var tapbackTargetGUID sql.NullString
	var tapbackEmoji sql.NullString

	err := res.Scan(
		&message.RowID, &message.GUID, &messageText, &dummyInt, &dummyText, &dummyInt, &messageSubject, &dummyText, attributedBody, &dummyInt,
		&dummyInt, &dummyText, &dummyText, &dummyText, &dummyInt, &message.Date, &message.DateRead, &dummyInt, &message.IsDelivered, &dummyInt,
		&message.IsEmote, &message.IsFromMe, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &message.IsSent, &dummyInt,
		&dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyText, &dummyInt, &dummyInt, &message.IsAudioMessage, &dummyInt,
		&dummyInt, &message.ItemType, &dummyInt, &newGroupTitle, &message.GroupActionType, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt,
		&dummyInt, &tapbackTargetGUID, &tapback.Type, &balloonBundleID, &dummyBlob, &dummyText, &dummyInt, &dummyInt, &dummyInt, messageSummaryInfo,
		&dummyInt, &dummyText, &dummyText, &dummyText, &dummyInt, &dummyText, &dummyText, &dummyInt, &dummyText, &dummyInt,
		&dummyInt, &dummyInt, &threadOriginatorGUID, &threadOriginatorPart, &dummyText, &dummyInt, &dummyInt, &dummyText, &message.DateRetracted, &message.DateEdited,
		&dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyText, &dummyInt, &dummyText, &tapbackEmoji, &dummyInt,
		&dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt,
		&message.ChatGUID, &message.ThreadID, &senderLocalID, &senderService, &targetLocalID, &targetService,
	)
	if err == nil {
		messageStringFields := map[*string]sql.NullString{
			&message.Text:              messageText,
			&message.NewGroupTitle:     newGroupTitle,
			&message.Subject:           messageSubject,
			&message.ReplyToGUID:       threadOriginatorGUID,
			&message.BalloonBundleID:   balloonBundleID,
			&message.Sender.LocalID:    senderLocalID,
			&message.Sender.Service:    senderService,
			&message.Target.LocalID:    targetLocalID,
			&message.Target.Service:    targetService,
			&tapback.TargetGUID:        tapbackTargetGUID,
			&tapback.Emoji:             tapbackEmoji,
			threadOriginatorPartString: threadOriginatorPart,
		}
		for field, value := range messageStringFields {
			if value.Valid {
				*field = value.String
			}
		}
	}
	return err
}

func OS14MessagesScan(res *sql.Rows, message *Message, attributedBody *[]byte, messageSummaryInfo *[]byte, tapback *Tapback, threadOriginatorPartString *string) error {
	var dummyText sql.NullString
	var dummyInt sql.NullInt64
	var dummyBlob []byte

	var messageText sql.NullString
	var messageSubject sql.NullString
	var newGroupTitle sql.NullString
	var threadOriginatorGUID sql.NullString
	var threadOriginatorPart sql.NullString
	var balloonBundleID sql.NullString
	var senderLocalID sql.NullString
	var senderService sql.NullString
	var targetLocalID sql.NullString
	var targetService sql.NullString
	var tapbackTargetGUID sql.NullString

	err := res.Scan(
		&message.RowID, &message.GUID, &messageText, &dummyInt, &dummyText, &dummyInt, &messageSubject, &dummyText, attributedBody, &dummyInt,
		&dummyInt, &dummyText, &dummyText, &dummyText, &dummyInt, &message.Date, &message.DateRead, &dummyInt, &message.IsDelivered, &dummyInt,
		&message.IsEmote, &message.IsFromMe, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &message.IsSent, &dummyInt,
		&dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyText, &dummyInt, &dummyInt, &message.IsAudioMessage, &dummyInt,
		&dummyInt, &message.ItemType, &dummyInt, &newGroupTitle, &message.GroupActionType, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt,
		&dummyInt, &tapbackTargetGUID, &tapback.Type, &balloonBundleID, &dummyBlob, &dummyText, &dummyInt, &dummyInt, &dummyInt, messageSummaryInfo,
		&dummyInt, &dummyText, &dummyText, &dummyText, &dummyInt, &dummyText, &dummyText, &dummyInt, &dummyText, &dummyInt,
		&dummyInt, &dummyInt, &threadOriginatorGUID, &threadOriginatorPart, &dummyText, &dummyInt, &dummyInt, &dummyText, &message.DateRetracted, &message.DateEdited,
		&dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyInt, &dummyText, &dummyInt, &dummyText,
		&message.ChatGUID, &message.ThreadID, &senderLocalID, &senderService, &targetLocalID, &targetService,
	)
	if err == nil {
		messageStringFields := map[*string]sql.NullString{
			&message.Text:              messageText,
			&message.NewGroupTitle:     newGroupTitle,
			&message.Subject:           messageSubject,
			&message.ReplyToGUID:       threadOriginatorGUID,
			&message.BalloonBundleID:   balloonBundleID,
			&message.Sender.LocalID:    senderLocalID,
			&message.Sender.Service:    senderService,
			&message.Target.LocalID:    targetLocalID,
			&message.Target.Service:    targetService,
			&tapback.TargetGUID:        tapbackTargetGUID,
			threadOriginatorPartString: threadOriginatorPart,
		}
		for field, value := range messageStringFields {
			if value.Valid {
				*field = value.String
			}
		}
	}

	return err
}

func GetMessagesScanFunctionForColumns(res *sql.Rows) (func(res *sql.Rows, message *Message, attributedBody *[]byte, messageSummaryInfo *[]byte, tapback *Tapback, threadOriginatorPartString *string) error, error) {
	columns, err := res.Columns()
	if err != nil {
		err = fmt.Errorf("getting columns for messages query: %w", err)
		return nil, err
	}
	// TODO: Actually check the columns are exactly as expected
	// TODO: Move Magic Numbers
	columnCount := len(columns)
	if columnCount == (95 + 6) {
		return OS16MessagesScan, nil
	} else if columnCount == (88 + 6) {
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
	// TODO: Actually check the columns are exactly as expected
	// TODO: Move Magic Numbers
	columnCount := len(columns)
	if columnCount == 27 {
		return OS16AttachmentScan, nil
	} else if columnCount == 24 {
		return OS14AttachmentScan, nil
	} else {
		return nil, fmt.Errorf("unrecognized column count (%d) in Message 'attachment' database", columnCount)
	}
}

func (c *MacOSMessagesClient) parseMessages(res *sql.Rows) ([]*Message, error) {
	messagesScanFunction, err := GetMessagesScanFunctionForColumns(res)
	if err != nil {
		return nil, fmt.Errorf("getting row scan function: %w", err)
	}
	messages := []*Message{}

	for res.Next() {
		var message Message
		var tapback Tapback
		var attributedBody []byte
		var messageSummaryInfo []byte

		var threadOriginatorPart string
		err = messagesScanFunction(res, &message, &attributedBody, &messageSummaryInfo, &tapback, &threadOriginatorPart)
		if err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}

		message.CreatedAt = time.Unix(AppleEpochUnix, message.Date)
		if message.DateRead != 0 {
			message.ReadAt = time.Unix(AppleEpochUnix, message.DateRead)
			message.IsRead = true
		}
		if message.DateEdited != 0 {
			message.EditedAt = time.Unix(AppleEpochUnix, message.DateEdited)
			message.IsEdited = true
		}
		if message.DateRetracted != 0 {
			message.RetractedAt = time.Unix(AppleEpochUnix, message.DateRetracted)
			message.IsRetracted = true
		}
		message.Attachments = make([]*Attachment, 0)
		attachmentRows, err := c.attachmentsQuery.Query(message.RowID)
		if err != nil {
			return nil, fmt.Errorf("querying attachments for %d: %w", message.RowID, err)
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
				return nil, fmt.Errorf("error scanning attachment row for %d: %w", message.RowID, err)
			}
			if len(stickerUserInfo) > 0 {
				plistDictionary := make(map[string]any, 0)
				if err := plist.NewDecoder(bytes.NewReader(stickerUserInfo)).Decode(plistDictionary); err != nil {
					return nil, fmt.Errorf("decoding plist to plistDictionary: %w", err)
				}
				pid, err := GetValueAsStringFromMapKey(plistDictionary, "pid")
				if err != nil {
					return nil, fmt.Errorf("finding pid key in plistDictionary: %w", err)
				}
				var pidAsInterface interface{} = *pid
				if stickerSource, ok := pidAsInterface.(StickerSource); !ok {
					attachment.StickerSource = stickerSource
				}
			}
			// TODO: add attribution_info parsing, meh
			message.Attachments = append(message.Attachments, &attachment)
		}
		if len(attributedBody) > 0 {
			if components, err := DecodeTypedStreamComponents(attributedBody); err != nil {
				c.log.Warn().Msgf("[%d] failed to decode attributedBody of %s: %v", message.RowID, message.GUID, err)
			} else {
				message.Components = components
				attributedBodyText := GetTextFromComponents(components)
				if attributedBodyText != nil {
					message.AttributedBodyText = *attributedBodyText
					if message.BalloonBundleID == "" {
						message.CombinedComponents = ConvertArchivablesToCombinedComponents(components, attributedBodyText)
					}
				}
			}
		}
		if len(messageSummaryInfo) > 0 {
			if editedMessageParts, err := EditedMessagePartsFromMessageSummaryInfo(messageSummaryInfo); err != nil {
				if message.IsEdited {
					return nil, fmt.Errorf("[%d] failed to convert message_summary_info to edited message parts: %v", message.RowID, err)
				}
			} else {
				if !message.IsEdited && len(editedMessageParts) > 1 {
					c.log.Warn().Msgf("[%d] message has message_summary_info of length %d but was not edited!", message.RowID, len(editedMessageParts))
				}
				for index, editedMessagePart := range editedMessageParts {
					if editedMessagePart.Status == EditedMessageStatusUnsent {
						retractedComponent := CombinedComponentRetraction{}
						if index >= len(message.CombinedComponents) {
							message.CombinedComponents = append(message.CombinedComponents, retractedComponent)
						} else {
							message.CombinedComponents = slices.Insert[[]CombinedComponent, CombinedComponent](message.CombinedComponents, index, retractedComponent)
						}
					}
				}
				message.EditedMessageParts = editedMessageParts
			}
		}
		err = nil
		if len(threadOriginatorPart) > 0 {
			// The thread_originator_part field seems to have three parts separated by colons.
			// The first two parts look like the part index, the third one is something else.
			// TODO this might not be reliable
			message.ReplyToPart, _ = strconv.Atoi(strings.Split(threadOriginatorPart, ":")[0])
		}
		if message.IsFromMe {
			message.Sender.LocalID = ""
		}
		if len(tapback.TargetGUID) > 0 {
			message.Tapback, err = tapback.Parse()
			if err != nil {
				c.log.Warn().Msgf("[%d] Failed to parse tapback in %s: %v", message.RowID, message.GUID, err)
			}
		}
		messages = append(messages, &message)
	}
	return messages, nil
}
