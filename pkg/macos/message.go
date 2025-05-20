package macos

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type Message struct {
	RowID         int
	ReplyToPart   int
	Date          int64
	DateRead      int64
	DateEdited    int64
	DateRetracted int64

	IsSent         bool
	IsFromMe       bool
	IsDelivered    bool
	IsEmote        bool
	IsAudioMessage bool
	IsRead         bool
	IsEdited       bool
	IsRetracted    bool

	GUID               string
	Subject            string
	Text               string
	AttributedBodyText string
	ChatGUID           string
	ReplyToGUID        string
	ThreadID           string
	NewGroupTitle      string
	BalloonBundleID    string

	Sender Identifier
	Target Identifier

	GroupActionType GroupActionType
	ItemType        ItemType

	CreatedAt   time.Time
	ReadAt      time.Time
	EditedAt    time.Time
	RetractedAt time.Time

	Attachments        []*Attachment
	Components         []Archivable
	CombinedComponents []CombinedComponent
	EditedMessageParts []*EditedMessagePart

	Tapback *Tapback
}

// TODO: Create a minimal "ParsediMessage" struct so we're not carrying around unused baggage all the way around
/*
ParsediMessage:
- ReplyToGUID
- ItemType
- CombinedComponents
- Attachments
- AttributedBodyText
- EditedMessageParts
- IsFromMe
- EditedAt
- CreatedAt
- IsPartEdited()
- CoonvertEditedMessagePart()
- GUID
- Tapback
- Sender

// TODO: Comb through CombinedComponent for TextEffects of TextEffectMention and add in the userIDs?
ConvertMessageToParts
- Message.ConvertAttributesToMessagePart
  - FormatTextRangeEffectsOnText
    - TextEffect.ApplyTextRangeEffectToText
*/

func (m Message) String() string {
	result := fmt.Sprintf("Row: %d\nSubject: %s\nText: %s\nAttributedBodyText: %s", m.RowID, m.Subject, m.Text, m.AttributedBodyText)
	result += fmt.Sprintf("\nAttachments: %d", len(m.Attachments))
	result += fmt.Sprintf("\nComponents: %d", len(m.Components))
	result += fmt.Sprintf("\nCombinedComponents: %d", len(m.CombinedComponents))
	result += fmt.Sprintf("\nEditedMessageParts: %d", len(m.EditedMessageParts))
	if len(m.EditedMessageParts) > 0 {
		for _, editedMessagePart := range m.EditedMessageParts {
			result += fmt.Sprintf("\n\t%s - %d edits", editedMessagePart.Status, len(editedMessagePart.EditHistory))
			if len(editedMessagePart.EditHistory) > 0 {
				for _, editHistory := range editedMessagePart.EditHistory {
					result += fmt.Sprintf("\n\t\t (%d) - %s", len(editHistory.Components), *editHistory.Text)
				}
			}
		}
	}
	if m.Tapback != nil {
		result += fmt.Sprintf("\nTapback: %s", m.Tapback.Emoji)
	}
	return result
}

type TapbackType int

const (
	TapbackLove TapbackType = iota + 2000
	TapbackLike
	TapbackDislike
	TapbackLaugh
	TapbackEmphasis
	TapbackQuestion
	TapbackEmoji
	TapbackSticker

	TapbackRemoveOffset = 1000
)

type Tapback struct {
	TargetGUID string
	Type       TapbackType
	Remove     bool
	TargetPart int
	Emoji      string
}

var (
	ErrUnknownNormalTapbackTarget = errors.New("unrecognized formatting of normal tapback target")
	ErrInvalidTapbackTargetPart   = errors.New("tapback target part index is invalid")
	ErrUnknownTapbackTargetType   = errors.New("unrecognized tapback target type")
)

func (t *Tapback) GetEmoji() string {
	switch t.Type {
	case 0:
		return ""
	case TapbackLove:
		return "\u2764\ufe0f" // "❤️"
	case TapbackLike:
		return "\U0001f44d\ufe0f" // "👍️"
	case TapbackDislike:
		return "\U0001f44e\ufe0f" // "👎️"
	case TapbackLaugh:
		return "\U0001f602" // "😂"
	case TapbackEmphasis:
		return "\u203c\ufe0f" // "‼️"
	case TapbackQuestion:
		return "\u2753\ufe0f" // "❓️"
	case TapbackEmoji:
		return t.Emoji
	default:
		return "\ufffd" // "�"
	}
}

func (tapback *Tapback) Parse() (*Tapback, error) {
	if tapback.Type >= 3000 && tapback.Type < 4000 {
		tapback.Type -= TapbackRemoveOffset
		tapback.Remove = true
	}
	if strings.HasPrefix(tapback.TargetGUID, "bp:") {
		tapback.TargetGUID = tapback.TargetGUID[len("bp:"):]
	} else if strings.HasPrefix(tapback.TargetGUID, "p:") {
		targetParts := strings.Split(tapback.TargetGUID[len("p:"):], "/")
		if len(targetParts) == 2 {
			var err error
			tapback.TargetPart, err = strconv.Atoi(targetParts[0])
			if err != nil {
				return nil, fmt.Errorf("%w: '%s' (%v)", ErrInvalidTapbackTargetPart, tapback.TargetGUID, err)
			}
			tapback.TargetGUID = targetParts[1]
		} else {
			return nil, fmt.Errorf("%w: '%s'", ErrUnknownNormalTapbackTarget, tapback.TargetGUID)
		}
	} else if len(tapback.TargetGUID) != 36 {
		return nil, fmt.Errorf("%w: '%s'", ErrUnknownTapbackTargetType, tapback.TargetGUID)
	}
	return tapback, nil
}

type StickerSource string

const (
	StickerSourceNone             StickerSource = ""
	StickerSourceGenmoji          StickerSource = "com.apple.messages.genmoji"
	StickerSourceAnimoji          StickerSource = "com.apple.Animoji.StickersApp.MessagesExtension"
	StickerSourceAnimojiJellyfish StickerSource = "com.apple.Jellyfish.Animoji"
	StickerSourceUserGenerated    StickerSource = "com.apple.Stickers.UserGenerated.MessagesExtension"
)

type Attachment struct {
	GUID                       string
	PathOnDisk                 string
	MimeType                   string
	FileName                   string
	IsSticker                  int
	StickerSource              StickerSource
	EmojiImageShortDescription string
}

func (a Attachment) Read() (result []byte, err error) {
	a.PathOnDisk, err = ReplaceHomeDirectory(a.PathOnDisk)
	if err != nil {
		return nil, fmt.Errorf("reading attachment: %w", err)
	}
	return os.ReadFile(a.PathOnDisk)
}

func (attachment *Attachment) GetMimeType() string {
	if attachment.MimeType == "" {
		mime, err := mimetype.DetectFile(attachment.PathOnDisk)
		if err != nil {
			return ""
		}
		attachment.MimeType = mime.String()
	}
	return attachment.MimeType
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

type ReadReceipt struct {
	ChatGUID   string
	ReadUpTo   string
	ReadAt     time.Time
	IsFromMe   bool
	SenderGUID string
}

type Identifier struct {
	LocalID string
	Service string
	IsGroup bool
}

func (m *Message) IsPartEdited(index int) bool {
	return len(m.EditedMessageParts) != 0 &&
		index < len(m.EditedMessageParts) &&
		m.EditedMessageParts[index].Status == EditedMessageStatusEdited
}

func (m *Message) ConvertAttributesToMessagePart(attributes []TextRangeEffect, index int) *bridgev2.ConvertedMessagePart {
	if m.IsPartEdited(index) {
		if len(m.EditedMessageParts) > 0 {
			return m.ConvertEditedMessagePart(index)
		}
	} else {
		convertedMessagePart := &bridgev2.ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgText,
				Body:    template.HTMLEscapeString(m.AttributedBodyText),
			},
		}
		formattedText := FormatTextRangeEffectsOnText(m.AttributedBodyText, attributes)
		// If we failed to parse any text above, make sure we sanitize it before using it
		if formattedText == "" {
			formattedText = convertedMessagePart.Content.Body
		}

		if strings.HasPrefix(formattedText, FITNESS_RECEIVER) {
			formattedText = strings.Replace(formattedText, FITNESS_RECEIVER, "", 1)
		}

		convertedMessagePart.Content.Format = event.FormatHTML
		convertedMessagePart.Content.FormattedBody = formattedText

		return convertedMessagePart
	}
	return nil
}

func (m *Message) ConvertMessageToParts(ctx context.Context, intent bridgev2.MatrixAPI, roomId id.RoomID) ([]*bridgev2.ConvertedMessagePart, error) {
	parts := []*bridgev2.ConvertedMessagePart{}
	if m.ItemType == 6 {
		parts = append(parts, ErrorToMessagePart(errors.New("unsupported item type (6: Shareplay)")))
		return parts, nil
	}
	if m.ItemType == 4 {
		parts = append(parts, ErrorToMessagePart(errors.New("unsupported item type (4: Location Sharing)")))
		return parts, nil
	}
	if m.BalloonBundleID != "" {
		parts = append(parts, m.ConvertAppMessageToMessagePart())
		return parts, nil
	}

	attachmentIndex := 0
	for componentIndex, combinedComponent := range m.CombinedComponents {
		switch component := combinedComponent.(type) {
		case CombinedComponentAttachment:
			if attachmentIndex < len(m.Attachments) {
				attachment := m.Attachments[attachmentIndex]
				convertedAttachment := attachment.ConvertAttachmentToConvertedMessagePart(ctx, intent, roomId, &component.AttachmentMeta)
				if attachment.IsSticker != 0 {
					// Could do "more" here: https://github.com/ReagentX/imessage-exporter/blob/develop/imessage-exporter/src/exporters/html.rs#L626
					switch attachment.StickerSource {
					case StickerSourceGenmoji:
						if attachment.EmojiImageShortDescription != "" {
							convertedAttachment.Content.Body += fmt.Sprintf(" [Genmoji prompt: %s]", attachment.EmojiImageShortDescription)
						}
					case StickerSourceAnimoji, StickerSourceAnimojiJellyfish:
						convertedAttachment.Content.Body += " [Animoji from Memoji]"
					case StickerSourceUserGenerated:
					case StickerSourceNone:
					}
				}
				parts = append(parts, convertedAttachment)
			} else {
				parts = append(parts, ErrorToMessagePart(errors.New("attachment does not exist")))
			}
		case CombinedComponentText:
			if len(m.AttributedBodyText) > 0 {
				if convertedMessage := m.ConvertAttributesToMessagePart(component.TextRangeEffects, componentIndex); convertedMessage != nil {
					parts = append(parts, convertedMessage)
				}
			}
		case CombinedComponentRetraction:
			if len(m.EditedMessageParts) != 0 {
				if convertedEditedMessagePart := m.ConvertEditedMessagePart(componentIndex); convertedEditedMessagePart != nil {
					parts = append(parts, convertedEditedMessagePart)
				}
			}
		default:
			panic(fmt.Sprintf("invalid type: %T", component))
		}
	}

	if len(parts) == 0 {
		// If no other combined components produced parts, add message text as a part
		if textPart := m.ConvertMessageText(); textPart != nil {
			parts = append(parts, textPart)
		}
	}

	for i, part := range parts {
		part.ID = networkid.PartID(strconv.Itoa(i))
	}

	return parts, nil
}

func (m *Message) ConvertAppMessageToMessagePart() *bridgev2.ConvertedMessagePart {
	// TODO: Literally anything
	// https://github.com/ReagentX/imessage-exporter/blob/develop/imessage-exporter/src/exporters/html.rs#L672
	return ErrorToMessagePart(errors.New("unsupported App message"))
}

func (m *Message) ConvertMessageText() *bridgev2.ConvertedMessagePart {
	// msg.Text = strings.ReplaceAll(msg.Text, "\ufffc", "")
	// msg.Subject = strings.ReplaceAll(msg.Subject, "\ufffc", "")

	if len(m.Text) == 0 && len(m.Subject) == 0 {
		return nil
	}
	part := &bridgev2.ConvertedMessagePart{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType: event.MsgText,
			Body:    m.Text,
		},
	}

	if len(m.Subject) != 0 {
		part.Content.Format = event.FormatHTML
		part.Content.FormattedBody = fmt.Sprintf("<strong>%s</strong><br>%s", event.TextToHTML(m.Subject), event.TextToHTML(m.Text))
		part.Content.Body = fmt.Sprintf("**%s**\n%s", m.Subject, m.Text)
	}
	return part
}

func (m *Message) ConvertEditedMessagePart(componentIndex int) *bridgev2.ConvertedMessagePart {
	editedMessageParts := m.EditedMessageParts

	if componentIndex >= len(editedMessageParts) {
		return nil
	}
	convertedMessagePart := &bridgev2.ConvertedMessagePart{
		Type: event.EventMessage,
	}
	editedMessagePart := editedMessageParts[componentIndex]
	switch editedMessagePart.Status {
	case EditedMessageStatusEdited:
		if len(editedMessagePart.EditHistory) < 1 {
			convertedMessagePart.Content = &event.MessageEventContent{
				MsgType: event.MsgNotice,
				Body:    "Message edited but contained no edit history",
			}
			break
		}
		finalEdit := editedMessagePart.EditHistory[len(editedMessagePart.EditHistory)-1]
		if finalEdit.Text == nil {
			convertedMessagePart.Content = &event.MessageEventContent{
				MsgType: event.MsgNotice,
				Body:    "Message edited but final edit contained no text",
			}
			break
		}
		text := *finalEdit.Text

		convertedMessagePart.Content = &event.MessageEventContent{
			MsgType: event.MsgText,
			Body:    template.HTMLEscapeString(text),
		}

		finalEditCombinedComponents := ConvertArchivablesToCombinedComponents(finalEdit.Components, finalEdit.Text)
		if len(finalEditCombinedComponents) > 0 {
			lastFinalEditCombinedComponent := finalEditCombinedComponents[len(finalEditCombinedComponents)-1]
			if combinedComponentText, ok := lastFinalEditCombinedComponent.(CombinedComponentText); ok {
				if len(combinedComponentText.TextRangeEffects) > 0 {
					convertedMessagePart.Content.Format = event.FormatHTML
					convertedMessagePart.Content.FormattedBody = FormatTextRangeEffectsOnText(text, combinedComponentText.TextRangeEffects)
					break
				}
			}
		}

	case EditedMessageStatusUnsent:
		who := "You"
		if !m.IsFromMe {
			who = "Sender"
		}
		suffix := "."
		if !m.EditedAt.IsZero() {
			if readableDateTimeDiff := dateTimeDiff(m.CreatedAt, m.EditedAt); readableDateTimeDiff != "" {
				suffix = fmt.Sprintf(" %s after sending%s", readableDateTimeDiff, suffix)
			}
		}
		convertedMessagePart.Content = &event.MessageEventContent{
			MsgType: event.MsgNotice,
			Body:    fmt.Sprintf("%s unsent this message part%s", who, suffix),
		}
		if !m.IsFromMe {
			username := "temp"
			server := "temp"
			name := "temp"
			convertedMessagePart.Content.Format = event.FormatHTML
			convertedMessagePart.Content.FormattedBody = fmt.Sprintf("%s unsent this message part%s", GetMentionText(username, server, name), suffix)
		}
	case EditedMessageStatusOriginal:
		return nil
	}
	return convertedMessagePart
}

func (a *Attachment) ConvertAttachmentToConvertedMessagePart(ctx context.Context, intent bridgev2.MatrixAPI, roomId id.RoomID, attachmentMeta *AttachmentMeta) *bridgev2.ConvertedMessagePart {
	attachmentData, err := a.Read()
	if err != nil {
		return ErrorToMessagePart(fmt.Errorf("reading attachment failed: %w", err))
	}
	mimeType := a.GetMimeType()
	fileName := a.FileName

	convertedMessagePart := &bridgev2.ConvertedMessagePart{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			Body: fileName,
			Info: &event.FileInfo{
				MimeType: mimeType,
				Size:     len(attachmentData),
			},
		},
	}

	url, file, err := intent.UploadMedia(ctx, roomId, attachmentData, fileName, mimeType)
	if err != nil {
		return ErrorToMessagePart(fmt.Errorf("%w: %w", bridgev2.ErrMediaReuploadFailed, err))
	}
	convertedMessagePart.Content.URL = url
	convertedMessagePart.Content.File = file

	switch {
	case strings.HasPrefix(mimeType, "image"):
		convertedMessagePart.Content.Info.Height = int(*attachmentMeta.Height)
		convertedMessagePart.Content.Info.Width = int(*attachmentMeta.Width)
		convertedMessagePart.Content.MsgType = event.MsgImage
	case strings.HasPrefix(mimeType, "video"):
		convertedMessagePart.Content.MsgType = event.MsgVideo
	case strings.HasPrefix(mimeType, "audio"):
		convertedMessagePart.Content.MsgType = event.MsgAudio
		if len(*attachmentMeta.Transcription) != 0 {
			convertedMessagePart.Content.Body += fmt.Sprintf(" | Transcript: %s", *attachmentMeta.Transcription)
		}
	default:
		convertedMessagePart.Content.MsgType = event.MsgFile
	}
	return convertedMessagePart
}
