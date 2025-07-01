package macos

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
	"unicode/utf16"

	"howett.net/plist"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type EffectType int64

const (
	Big     EffectType = 5
	Small   EffectType = 11
	Shake   EffectType = 9
	Nod     EffectType = 8
	Explode EffectType = 12
	Ripple  EffectType = 4
	Bloom   EffectType = 6
	Jitter  EffectType = 10
)

var EFFECT_TYPE_STRING_MAP = map[EffectType]string{
	Big:     "big",
	Small:   "small",
	Shake:   "shake",
	Nod:     "nod",
	Explode: "explode",
	Ripple:  "ripple",
	Bloom:   "bloom",
	Jitter:  "jitter",
}

func (e *EffectType) String() (result string) {
	if result, ok := EFFECT_TYPE_STRING_MAP[*e]; ok {
		return result
	}
	return fmt.Sprintf("unknown-%d", *e)
}

func (e *EffectType) IsValid() bool {
	switch *e {
	case Big, Small, Shake, Nod, Explode, Ripple, Bloom, Jitter:
		return true
	default:
		return false
	}
}

type ConversionType int

const (
	ConversionTypeCurrency ConversionType = iota
	ConversionTypeDistance
	ConversionTypeTemperature
	ConversionTypeTimezone
	ConversionTypeVolume
	ConversionTypeWeight
)

type Identifier struct {
	LocalID string
	Service string
	IsGroup bool
}

type AttachmentMeta struct {
	GUID          *string
	Transcription *string
	Height        *float64
	Width         *float64
	Name          *string
}

type Message struct {
	DBDate int64

	Text            string
	Subject         string
	GUID            string
	BalloonBundleID string
	NewGroupTitle   string
	ReplyToGUID     string
	ChatGUID        string
	OtherID         string
	HandleID        string

	DBRowID     int
	ReplyToPart int

	IsSent      bool
	IsRead      bool
	IsEdited    bool
	IsRetracted bool
	IsFromMe    bool

	CreatedAt   time.Time
	ReadAt      time.Time // ?
	EditedAt    time.Time // ?
	RetractedAt time.Time // ?

	AttributedString   NSMutableAttributedString
	EditedMessageParts []*EditedMessagePart
	Tapback            *Tapback
	ItemType           ItemType
	GroupActionType    GroupActionType

	PayloadData []byte

	Attachments map[string]*Attachment
}

func (m Message) String() string {
	results := []string{}
	results = append(results, fmt.Sprintf("DB Row: %d", m.DBRowID))
	results = append(results, fmt.Sprintf("Type: %d", m.ItemType))
	results = append(results, fmt.Sprintf("Handler ID: %s", m.HandleID))
	results = append(results, fmt.Sprintf("Other ID: %s", m.OtherID))
	if len(m.Subject) > 0 {
		results = append(results, fmt.Sprintf("Subject: %s", m.Subject))
	}
	if len(m.Text) > 0 {
		results = append(results, fmt.Sprintf("Text: %s", m.Text))
	}

	results = append(results, fmt.Sprintf("Attachments: %d", len(m.Attachments)))
	for guid, attachment := range m.Attachments {
		results = append(results, fmt.Sprintf("\t%s - %s", guid, attachment.GUID))
	}

	results = append(results, m.AttributedString.String())

	results = append(results, fmt.Sprintf("EditedMessageParts: %d", len(m.EditedMessageParts)))
	if len(m.EditedMessageParts) > 0 {
		for _, editedMessagePart := range m.EditedMessageParts {
			results = append(results, fmt.Sprintf("\t%s - %d edits", editedMessagePart.Status, len(editedMessagePart.EditHistory)))
			if len(editedMessagePart.EditHistory) > 0 {
				for _, editHistory := range editedMessagePart.EditHistory {
					results = append(results, fmt.Sprintf("\t\t (%d ranges) - %s", len(editHistory.AttributedString.RangedAttributes), *editHistory.Text))
				}
			}
		}
	}
	if m.Tapback != nil {
		if m.Tapback.Emoji != "" {
			results = append(results, fmt.Sprintf("Tapback: %s", m.Tapback.Emoji))
		}
		if m.Tapback.TargetGUID != "" {
			results = append(results, fmt.Sprintf("Tapback Target GUID: %s", m.Tapback.TargetGUID))
		}
		results = append(results, fmt.Sprintf("Tapback Type: %d", m.Tapback.Type))
		results = append(results, fmt.Sprintf("Tapback Remove: %t", m.Tapback.Remove))
		results = append(results, fmt.Sprintf("Tapback Target Part: %d", m.Tapback.TargetPart))
	}
	return strings.Join(results, "\n")
}

func (m *Message) ConvertMessageToParts(ctx context.Context, intent bridgev2.MatrixAPI, roomID id.RoomID, get_users func() ([]id.UserID, error)) ([]*bridgev2.ConvertedMessagePart, error) {
	if m.BalloonBundleID != "" {
		if parts, err := m.ConvertAppMessageToMessageParts(ctx, intent, roomID); err != nil {
			return []*bridgev2.ConvertedMessagePart{ErrorToMessagePart(err)}, nil
		} else {
			return parts, nil
		}
	}

	convertedMessageParts, err := m.ConvertAttributedStringToFormattedHTMLParts(ctx, intent, roomID, get_users)
	if err != nil {
		return []*bridgev2.ConvertedMessagePart{ErrorToMessagePart(fmt.Errorf("converting AttributedString to message parts: %w", err))}, nil
	}

	if len(convertedMessageParts) == 0 {
		// If no parts were produced, add message text as a part
		if textPart := m.ConvertMessageText(); textPart != nil {
			convertedMessageParts = append(convertedMessageParts, textPart)
		}
	}

	return convertedMessageParts, nil
}

func (m *Message) ConvertAttributedStringToFormattedHTMLParts(ctx context.Context, intent bridgev2.MatrixAPI, roomID id.RoomID, get_users func() ([]id.UserID, error)) ([]*bridgev2.ConvertedMessagePart, error) {
	a := m.AttributedString
	parts := []*bridgev2.ConvertedMessagePart{}
	currentMessageUTF16Index := 0
	currentMessageAttributePart := int64(0)
	messageStringUTF16 := utf16.Encode([]rune(a.Value))

	currentMessagePart := &bridgev2.ConvertedMessagePart{
		Content: &event.MessageEventContent{
			Body:     "",
			Mentions: &event.Mentions{},
		},
	}

	if parts, err := m.CreateURLPreview(); err == nil {
		return parts, nil
	}

	for _, rangedAttribute := range a.RangedAttributes {
		attributes := rangedAttribute.AttributeMap

		substring := messageStringUTF16[currentMessageUTF16Index:(rangedAttribute.length + currentMessageUTF16Index)]
		currentMessageUTF16Index += rangedAttribute.length
		decodedSubstring := string(utf16.Decode(substring))
		cleanSubstring := strings.ReplaceAll(decodedSubstring, "\ufffc", "")

		if messageAttributePart, ok := attributes[MessagePartAttributeName]; ok {
			if messageAttributePartAsInt, ok := messageAttributePart.(*int64); !ok {
				return nil, fmt.Errorf("message attribute part was not an int")
			} else if currentMessageAttributePart != *messageAttributePartAsInt {
				parts = append(parts, currentMessagePart)
				currentMessageAttributePart = *messageAttributePartAsInt
				currentMessagePart = &bridgev2.ConvertedMessagePart{
					Content: &event.MessageEventContent{
						Body:     "",
						Mentions: &event.Mentions{},
					},
				}
			}
		} else {
			// All attribute ranges seem to contain a part so this might be a legitimate spot to error out?
		}

		currentMessagePart.Content.Body += cleanSubstring
		currentMessagePart.Content.Format = event.FormatHTML

		if fileGUID, ok := attributes[FileTransferGUIDAttributeName]; ok {
			if fileGUIDString, ok := fileGUID.(*string); ok {
				if attachment, ok := m.Attachments[*fileGUIDString]; ok {
					attachmentMeta := &AttachmentMeta{}
					if audioTranscriptionValue, ok := attributes[AudioTranscription]; ok {
						if audioTransciption, ok := audioTranscriptionValue.(*string); ok {
							attachmentMeta.Transcription = audioTransciption
						}
					}
					if heightValue, ok := attributes[InlineMediaHeightAttributeName]; ok {
						if height, ok := heightValue.(*float64); ok {
							attachmentMeta.Height = height
						}
					}
					if widthValue, ok := attributes[InlineMediaWidthAttributeName]; ok {
						if width, ok := widthValue.(*float64); ok {
							attachmentMeta.Width = width
						}
					}
					currentMessagePart = attachment.ConvertAttachmentToConvertedMessagePart(ctx, intent, roomID, attachmentMeta)

					if attachment.IsSticker != 0 {
						// Could do "more" here: https://github.com/ReagentX/imessage-exporter/blob/develop/imessage-exporter/src/exporters/html.rs#L626
						switch attachment.StickerSource {
						case StickerSourceGenmoji:
							if attachment.EmojiImageShortDescription != "" {
								currentMessagePart.Content.Body += fmt.Sprintf(" [Genmoji prompt: %s]", attachment.EmojiImageShortDescription)
							}
						case StickerSourceAnimoji, StickerSourceAnimojiJellyfish:
							currentMessagePart.Content.Body += " [Animoji from Memoji]"
						case StickerSourceUserGenerated:
						case StickerSourceNone:
						}
					}
				} else {
					currentMessagePart = ErrorToMessagePart(fmt.Errorf("file GUID (%s) from message part was not found in attachments", *fileGUIDString))
				}
				// Nit: In theory an attachment message part _could_ have other styled content with it and this will skip that
				continue
			}
		}

		formattedSubstring := cleanSubstring

		if _, ok := attributes[TextBoldAttributeName]; ok {
			formattedSubstring = fmt.Sprintf("<b>%s</b>", formattedSubstring)
		}
		if _, ok := attributes[TextUnderlineAttributeName]; ok {
			formattedSubstring = fmt.Sprintf("<u>%s</u>", formattedSubstring)
		}
		if _, ok := attributes[TextItalicAttributeName]; ok {
			formattedSubstring = fmt.Sprintf("<i>%s</i>", formattedSubstring)
		}
		if _, ok := attributes[TextStrikethroughAttributeName]; ok {
			formattedSubstring = fmt.Sprintf("<s>%s</s>", formattedSubstring)
		}

		if linkValue, ok := attributes[LinkAttributeName]; ok {
			linkAddress := "Unable to convert link to string"
			if link, ok := linkValue.(*string); ok && link != nil {
				linkAddress = *link
			}
			formattedSubstring = fmt.Sprintf("<a href=\"%s\">%s</a>", linkAddress, formattedSubstring)
		}

		if strings.HasPrefix(formattedSubstring, FITNESS_RECEIVER) {
			formattedSubstring = strings.Replace(formattedSubstring, FITNESS_RECEIVER, "", 1)
		}

		if calendarValue, ok := attributes[CalendarEventAttributeName]; ok {
			if calendarPlistBytes, ok := calendarValue.([]byte); !ok {
				return nil, fmt.Errorf("calendar attribute could not be coerced to bytes: %f", calendarValue)
			} else {
				calendarPlistDictionary := make(map[string]any, 0)
				if err := plist.NewDecoder(bytes.NewReader(calendarPlistBytes)).Decode(calendarPlistDictionary); err != nil {
					return nil, fmt.Errorf("decoding plist to calendarPlistDictionary: %w", err)
				}
				if objects, ok := calendarPlistDictionary["$objects"]; !ok {
					return nil, fmt.Errorf("calendar plist did not contain objects list")
				} else {
					if objectsAsList, ok := objects.([]any); !ok {
						return nil, fmt.Errorf("objects was not coercable to list: %f", objects)
					} else {
						for j, object := range objectsAsList {
							if objectString, ok := object.(string); ok && objectString == "DateTime" {
								previousObject := objectsAsList[j-1]
								if previousObjectString, ok := previousObject.(string); ok {
									if eventTime, err := BestEffortDateTimeParse(previousObjectString, m.CreatedAt); err == nil {
										ics := TimeToICS(eventTime)
										icsBase64 := base64.URLEncoding.EncodeToString([]byte(ics))
										href := fmt.Sprintf("data:text/calendar;base64,%s", icsBase64)
										formattedSubstring = fmt.Sprintf("<a href=\"%s\">%s</a>", href, formattedSubstring)
									}
								}
							}
						}
					}
				}
			}
		}

		if _, ok := attributes[OneTimeCodeAttributeName]; ok {
			// TODO: is a copy-able html element a thing yet?
			formattedSubstring = fmt.Sprintf("<pre>%s</pre>", formattedSubstring)
		}

		if effectValue, ok := attributes[TextEffectAttributeName]; ok {
			effect := ""
			if effectInt, ok := effectValue.(*int64); ok {
				effectType := EffectType(*effectInt)
				effect = effectType.String()
			} else {
				effect = fmt.Sprintf("wrong-type-%T", effectValue)
			}
			formattedSubstring = fmt.Sprintf("<span class=\"%s\">%s</span>", effect, formattedSubstring)
		}

		if mentionValue, ok := attributes[MentionConfirmedMention]; ok {
			if mentionString, ok := mentionValue.(*string); ok {
				// Only want to call this if we have to? Except we'll call it for each mention...
				// Better than for every message in any case
				if users, err := get_users(); err == nil {
					for _, user := range users {
						if strings.Contains(string(user), *mentionString) {
							mentionID := string(user)
							mentionURL := fmt.Sprintf("https://matrix.to/#/%s", mentionID)
							formattedSubstring = fmt.Sprintf("<a href=\"%s\">%s</a>", mentionURL, formattedSubstring)
							currentMessagePart.Content.Mentions.Add(id.UserID(mentionID))
							break
						}
					}
				}
			}
		}

		currentMessagePart.Content.FormattedBody += formattedSubstring

		/* TODO:
		MoneyAttributeName
		DataDetectedAttributeName
		PhoneNumberAttributeName
		AddressAttributeName
		*/
	}
	parts = append(parts, currentMessagePart)

	return parts, nil
}

func (m *Message) ConvertAppMessageToMessageParts(ctx context.Context, intent bridgev2.MatrixAPI, roomID id.RoomID) ([]*bridgev2.ConvertedMessagePart, error) {
	bundleID := m.BalloonBundleID
	if strings.Contains(bundleID, ":") {
		parts := strings.Split(bundleID, ":")
		if len(parts) != 3 {
			return nil, fmt.Errorf("undexpected number of parts in balloon bundle ID: %d (%s)", len(parts), bundleID)
		}
		bundleID = parts[2]
	}

	if bundleID == "com.apple.messages.URLBalloonProvider" {
		return m.CreateURLPreview()
	}

	if bundleID == "com.apple.gamecenter.GameCenterUIService.GameCenterMessageExtension" {
		return m.ConvertFirstBreadcrumbAndAttachmentToMessageParts(ctx, intent, roomID, "Game Center")
	}

	if bundleID == "com.americanexpress.amexservice.imessage" {
		return m.ConvertFirstBreadcrumbAndAttachmentToMessageParts(ctx, intent, roomID, "American Express Service")
	}

	if bundleID == "com.apple.findmy.FindMyMessagesApp" {
		return m.ConvertFirstBreadcrumbToMessageParts("Find My")
	}

	if bundleID == "com.apple.messages.chatbot" {
		return m.ConvertChatBotMessageToParts(ctx, intent, roomID)
	}

	// TODO: Other bundle IDs:
	// com.apple.messages.chatbot
	// com.apple.Handwriting.HandwritingProvider
	// com.apple.DigitalTouchBalloonProvider
	// com.apple.PassbookUIService.PeerPaymentMessagesExtension
	// com.apple.ActivityMessagesApp.MessagesExtension
	// com.apple.mobileslideshow.PhotosMessagesApp
	// com.apple.SafetyMonitorApp.SafetyMonitorMessages
	// com.apple.findmy.FindMyMessagesApp
	// https://github.com/ReagentX/imessage-exporter/blob/0ce28702ef58c3eef40b96cc4dc3b80ed84138e8/imessage-exporter/src/exporters/html.rs#L687
	return nil, fmt.Errorf("unsupported App message: %s", bundleID)
}

func (m *Message) CreateURLPreview() ([]*bridgev2.ConvertedMessagePart, error) {
	if len(m.AttributedString.RangedAttributes) == 1 {
		if _, ok := m.AttributedString.RangedAttributes[0].AttributeMap[LinkAttributeName]; ok && m.PayloadData != nil {
			if flatPlistData, err := FlatObjectMapFromPlistData(m.PayloadData, "root"); err == nil {
				if urlPreview, err := URLPreviewFromFlatPlistData(flatPlistData); err == nil {
					return []*bridgev2.ConvertedMessagePart{{
						Content: &event.MessageEventContent{
							Body:          m.AttributedString.Value,
							Format:        event.FormatHTML,
							FormattedBody: urlPreview,
							Mentions:      &event.Mentions{},
						},
					}}, nil
				} else {
					return nil, fmt.Errorf("creating URL preview from plist data: %w", err)
				}
			} else {
				return nil, fmt.Errorf("getting object map from payload plist data: %w", err)
			}
		} else {
			return nil, fmt.Errorf("first attributed string range did not contain link (%t) or payload was null (%t)", !ok, m.PayloadData == nil)
		}
	} else {
		return nil, fmt.Errorf("more than one attributed string range in message: %d ranges", len(m.AttributedString.RangedAttributes))
	}
}

func (m *Message) ConvertFirstBreadcrumbAndAttachmentToMessageParts(ctx context.Context, intent bridgev2.MatrixAPI, roomID id.RoomID, messageType string) ([]*bridgev2.ConvertedMessagePart, error) {
	if len(m.Attachments) == 0 {
		return nil, fmt.Errorf("no attachments in %s message", messageType)
	}

	var attachmentIDValue, messageValue any
	attachmentID := ""
	message := ""
	var ok bool
	for _, attributes := range m.AttributedString.RangedAttributes {
		if attachmentIDValue, ok = attributes.AttributeMap[FileTransferGUIDAttributeName]; ok {
			if attachmentIDString, ok := attachmentIDValue.(*string); ok {
				attachmentID = *attachmentIDString
			}
		}
		if messageValue, ok = attributes.AttributeMap[BreadcrumbTextMarkerAttributeName]; ok {
			if messageString, ok := messageValue.(*string); ok {
				message = *messageString
			}
		}
	}

	if attachmentID == "" {
		return nil, fmt.Errorf("no attahment ID found in %s message", messageType)
	}
	if message == "" {
		return nil, fmt.Errorf("no message found in %s message", messageType)
	}

	if attachment, ok := m.Attachments[attachmentID]; ok {
		currentMessagePart := attachment.ConvertAttachmentToConvertedMessagePart(ctx, intent, roomID, &AttachmentMeta{})
		currentMessagePart.Content.Body = fmt.Sprintf("%s: %s", messageType, message)
		return []*bridgev2.ConvertedMessagePart{currentMessagePart}, nil
	} else {
		return nil, fmt.Errorf("attachment was not found in %s message", messageType)
	}
}

func (m *Message) ConvertFirstBreadcrumbToMessageParts(messageType string) ([]*bridgev2.ConvertedMessagePart, error) {
	var messageValue any
	message := ""
	var ok bool
	for _, attributes := range m.AttributedString.RangedAttributes {
		if messageValue, ok = attributes.AttributeMap[BreadcrumbTextMarkerAttributeName]; ok {
			if messageString, ok := messageValue.(*string); ok {
				message = *messageString
			}
		}
	}

	if message == "" {
		return nil, fmt.Errorf("no message found in %s message", messageType)
	}

	return []*bridgev2.ConvertedMessagePart{{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType: event.MsgText,
			Body:    fmt.Sprintf("%s: %s", messageType, message),
		},
	}}, nil
}

func (m *Message) ConvertChatBotMessageToParts(ctx context.Context, intent bridgev2.MatrixAPI, roomID id.RoomID) ([]*bridgev2.ConvertedMessagePart, error) {
	parts := []*bridgev2.ConvertedMessagePart{}

	for _, attribute := range m.AttributedString.RangedAttributes {
		transferMap := map[string]any{}
		cards := []any{}
		if transferMapValue, ok := attribute.AttributeMap[URLToTransferMapAttributeName]; ok {
			if transferMapMap, ok := transferMapValue.(map[string]any); ok {
				transferMap = transferMapMap
			}
		}
		if cardsValue, ok := attribute.AttributeMap[RichCardsAttributeName]; ok {
			if cardsValueArray, ok := cardsValue.([]any); ok {
				cards = cardsValueArray
			}
		}
		for _, card := range cards {
			if cardMap, ok := card.(map[string]any); ok {
				parts = append(parts, m.ConvertCardToMessageParts(ctx, intent, roomID, cardMap, transferMap)...)
			}
		}
	}

	return parts, nil
}

func (m *Message) ConvertCardToMessageParts(ctx context.Context, intent bridgev2.MatrixAPI, roomID id.RoomID, card map[string]any, transferMap map[string]any) []*bridgev2.ConvertedMessagePart {
	parts := []*bridgev2.ConvertedMessagePart{}

	if media, err := GetValueAsTypeFromMapKey[map[string]any](card, "media"); err == nil {
		if mediaUrl, err := GetValueAsTypeFromMapKey[*string](*media, "mediaUrl"); err == nil {
			if attachmentGUIDValue, ok := transferMap[**mediaUrl]; ok {
				if attachmentGUID, ok := attachmentGUIDValue.(*string); ok {
					if attachment, ok := m.Attachments[*attachmentGUID]; ok {
						parts = append(parts, attachment.ConvertAttachmentToConvertedMessagePart(ctx, intent, roomID, &AttachmentMeta{}))
					}
				}
			}
		}
	}

	m.Subject = "No title found"
	m.Text = "No card description found"
	if titleString, err := GetValueAsTypeFromMapKey[*string](card, "title"); err == nil {
		m.Subject = **titleString
	}
	if cardDescriptionString, err := GetValueAsTypeFromMapKey[*string](card, "cardDescription"); err == nil {
		m.Text = **cardDescriptionString
	}

	parts = append(parts, m.ConvertMessageText())

	return parts
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
