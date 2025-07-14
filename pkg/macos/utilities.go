package macos

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"maps"

	"github.com/nyaruka/phonenumbers"
	"github.com/tj/go-naturaldate"
	"golang.org/x/image/draw"
	"howett.net/plist"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
)

const FITNESS_RECEIVER = "$(kIMTranscriptPluginBreadcrumbTextReceiverIdentifier)"
const PORTAL_ID_SEPARATOR = "|"

var AppleEpoch = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
var AppleEpochUnix = AppleEpoch.Unix()
var AppleEpochUnixNano = AppleEpoch.UnixNano()

const (
	COLOR_CLEAR = "\x1b[0m"
)

func isASCIIDigit(input byte) bool {
	return input <= 57 && input >= 48
}

func byteToDigit(input byte) uint8 {
	return input - 48
}

func MakeMessagesPortalID(userLoginID networkid.UserLoginID, chatHandlesIDs string) networkid.PortalID {
	return networkid.PortalID(strings.Join([]string{"MessagesID", string(userLoginID), chatHandlesIDs}, PORTAL_ID_SEPARATOR))
}

func ChatHandlesIDsFromPortalID(portalID networkid.PortalID) string {
	parts := strings.Split(string(portalID), PORTAL_ID_SEPARATOR)
	if len(parts) != 3 {
		return ""
	}
	return parts[2]
}

func RunOsascript(script string, args ...string) (string, string, error) {
	args = append([]string{"-"}, args...)
	cmd := exec.Command("osascript", args...)

	var errorBuf bytes.Buffer
	var outBuf bytes.Buffer
	cmd.Stderr = &errorBuf
	cmd.Stdout = &outBuf

	// Make sure Wait is always called even if something fails.
	defer func() {
		go func() {
			_ = cmd.Wait()
		}()
	}()

	var stdin io.WriteCloser
	var err error
	if stdin, err = cmd.StdinPipe(); err != nil {
		err = fmt.Errorf("failed to open stdin pipe: %w", err)
	} else if err = cmd.Start(); err != nil {
		err = fmt.Errorf("failed to run osascript: %w", err)
	} else if _, err = io.WriteString(stdin, script); err != nil {
		err = fmt.Errorf("failed to send script to osascript: %w", err)
	} else if err = stdin.Close(); err != nil {
		err = fmt.Errorf("failed to close stdin pipe: %w", err)
	} else if err = cmd.Wait(); err != nil {
		err = fmt.Errorf("failed to wait for osascript: %w (stderr: %s)", err, strings.TrimSpace(errorBuf.String()))
	} else if cmd.ProcessState.ExitCode() != 0 {
		err = fmt.Errorf("exit code: %d", cmd.ProcessState.ExitCode())
	}
	return outBuf.String(), errorBuf.String(), err
}

func GetImageFromVCard(vcard string) ([]byte, error) {
	collectString := false
	collectedString := ""
	for _, line := range strings.Split(vcard, "\n") {
		if collectString {
			if !strings.HasPrefix(line, " ") {
				break
			}
			collectedString += strings.TrimSpace(line)

			continue
		}

		if strings.HasPrefix(line, "PHOTO;") {
			collectString = true
			firstLineParts := strings.Split(line, ":")
			collectedString += firstLineParts[1]
		}
	}
	if len(collectedString) == 0 {
		return nil, fmt.Errorf("did not find a photo in vcard")
	}
	return base64.StdEncoding.DecodeString(collectedString)
}

func SupplementMemberMapWithContactsMap(memberMap *map[networkid.UserID]bridgev2.ChatMember, contactsMap map[networkid.UserID]ContactInformation, contactsClient MacOSContactsClient) {
	for memberKey, member := range *memberMap {
		if contactInformation, ok := contactsMap[memberKey]; ok {
			SupplementChatMemberWithContactInformation(&member, contactInformation, contactsClient)
		}
	}
}

func SupplementChatMemberWithContactInformation(member *bridgev2.ChatMember, contactInformation ContactInformation, contactsClient MacOSContactsClient) {
	member.Nickname = &contactInformation.Nickname
	SupplementUserInfoWithContactInformation(member.UserInfo, contactInformation, contactsClient)
}

func SupplementUserInfoWithContactInformation(userInfo *bridgev2.UserInfo, contactInformation ContactInformation, contactsClient MacOSContactsClient) {
	name := FullName(contactInformation.FirstName, contactInformation.LastName)
	userInfo.Name = &name
	userInfo.Avatar = contactsClient.GetWrappedAvatarForID(contactInformation.ID)
	if name != "" {
		userInfo.Identifiers = append(userInfo.Identifiers, name)
	}
	if contactInformation.Nickname != "" {
		userInfo.Identifiers = append(userInfo.Identifiers, contactInformation.Nickname)
	}
}

func FullName(firstName string, lastName string) string {
	return fmt.Sprintf("%s %s", firstName, lastName)
}

func ParseFormatPhoneNumber(phoneNumber string, countryCode string) (*networkid.UserID, error) {
	if !strings.HasPrefix(phoneNumber, "+") {
		phoneNumber = "+1" + phoneNumber
	}
	if num, err := phonenumbers.Parse(phoneNumber, countryCode); err != nil {
		return nil, fmt.Errorf("parsing phone number [%s] and country code [%s]: %w", phoneNumber, countryCode, err)
	} else {
		userID := networkid.UserID(phonenumbers.Format(num, phonenumbers.E164))
		return &userID, nil
	}
}

func ReplaceHomeDirectory(input string, home string) string {
	if strings.HasPrefix(input, "~/") {
		return filepath.Join(home, input[2:])
	}
	return input
}

// https://github.com/tinkerator/xxd/blob/main/xxd.go
func Dump(data []byte, colors []byte) (lines []string) {
	offset := 0 & 15
	base := 0 - offset
	index := 0
	for n := len(data); n > 0; offset, base = 0, base+16 {
		parts := []string{fmt.Sprintf("%08x:", base)}
		count := 16 - offset
		if count > n {
			count = n
		}
		ch := make([]byte, 17)
		ch[0] = ' '
		for i := 1; i <= offset; i++ {
			parts = append(parts, "  ")
			ch[i] = byte(' ')
		}
		for i := range count {
			c := data[index+i]
			color := colors[index+i]
			parts = append(parts, fmt.Sprintf("\x1b[%dm%02x%s", color, c, COLOR_CLEAR))
			if c < 32 || c >= 127 {
				c = byte('.')
			}
			ch[1+i+offset] = c
		}
		for i := offset + count; i < 16; i++ {
			parts = append(parts, "  ")
		}
		parts = append(parts, string(ch[:1+offset+count]))
		lineContent := strings.Join(parts, " ") + COLOR_CLEAR
		lines = append(lines, lineContent)
		index += count
		n -= count
	}
	return
}

func ErrorToMessagePart(err error) *bridgev2.ConvertedMessagePart {
	return &bridgev2.ConvertedMessagePart{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType: event.MsgNotice,
			Body:    err.Error(),
		},
		Extra: map[string]any{
			"unsupported": true,
		},
	}
}

/*
func dateTimeDiff(start time.Time, end time.Time) string {
	durationSecondsRound := end.Sub(start).Round(time.Second)
	if durationSecondsRound < 0 {
		return ""
	}
	return humanizeDuration(durationSecondsRound)
}

// https://gist.github.com/harshavardhana/327e0577c4fed9211f65?permalink_comment_id=2366908#gistcomment-2366908
func humanizeDuration(duration time.Duration) string {
	days := int64(duration.Hours() / 24)
	hours := int64(math.Mod(duration.Hours(), 24))
	minutes := int64(math.Mod(duration.Minutes(), 60))
	seconds := int64(math.Mod(duration.Seconds(), 60))

	chunks := []struct {
		singularName string
		amount       int64
	}{
		{"day", days},
		{"hour", hours},
		{"minute", minutes},
		{"second", seconds},
	}

	parts := []string{}

	for _, chunk := range chunks {
		switch chunk.amount {
		case 0:
			continue
		case 1:
			parts = append(parts, fmt.Sprintf("%d %s", chunk.amount, chunk.singularName))
		default:
			parts = append(parts, fmt.Sprintf("%d %ss", chunk.amount, chunk.singularName))
		}
	}

	return strings.Join(parts, " ")
}
*/

func ConvertConvertedMessageToString(c *bridgev2.ConvertedMessage) string {
	result := fmt.Sprintf("Converted:\n%d parts:\n", len(c.Parts))
	for _, part := range c.Parts {
		result += fmt.Sprintf(" - %s\n   Body(%d): %s\n   Format: %s\n   FormattedBody(%d): %s\n", part.Type, len(part.Content.Body), part.Content.Body, part.Content.Format, len(part.Content.FormattedBody), part.Content.FormattedBody)
	}
	return result
}

func ConvertEditToString(c *bridgev2.ConvertedEdit) string {
	result := fmt.Sprintf("ConvertedEdit:\nDeleted: %d\nModified: %d\nNew: %d", len(c.DeletedParts), len(c.ModifiedParts), len(c.AddedParts.Parts))
	for _, d := range c.DeletedParts {
		result += fmt.Sprintf("\n\tD - %s", d.ID)
	}
	for _, m := range c.ModifiedParts {
		result += fmt.Sprintf("\n\tM - %s", m.Type)
	}
	for _, a := range c.AddedParts.Parts {
		result += fmt.Sprintf("\n\tA - %s", a.Type)
	}
	return result
}

func GetMentionText(username string, server string, name string) string {
	return fmt.Sprintf("<a href=\"https://matrix.to/#/@%s:%s\">@%s</a>", username, server, name)
}

var DATE_TIME_LAYOUTS = [...]string{
	"01/02/2006 15:04:05 PM",
	"1/2/2006 15:04:05 PM",
	"01/02/2006 15:04 PM",
	"01/02/2006 15:04 pm",
	"01/02/2006 15:04PM",
	"01/02/06 15:04 PM",
	"01/02/06 15:04PM",
	"1/2/2006 15:04 PM",
	"1/2/06 15:04:05 MST",
	"2006/01/02 15:04 PM",
	"2006/01/02 15:04PM",
	"15:04PM 02/01/2006",
	"15:04PM 2006/01/02",
	"15:04 PM 01/02",
	"15:04 pm 01/02",
	"1/2 15:04 PM MST",
	"1/2 15 am",
	"15pm Monday 1/2",
	"Monday 1/2 15PM",
	time.RFC3339,
}

func BestEffortDateTimeParse(input string, ref time.Time) (time.Time, error) {
	sanitized := strings.ReplaceAll(input, "@", " ")
	sanitized = strings.ReplaceAll(sanitized, "-", " ")
	sanitized = strings.ReplaceAll(sanitized, ",", "")
	sanitized = strings.ReplaceAll(sanitized, ".", "")
	sanitized = strings.TrimSpace(sanitized)
	if eventTime, err := naturaldate.Parse(sanitized, ref, naturaldate.WithDirection(naturaldate.Future)); err == nil {
		return eventTime, nil
	}
	sanitized = strings.ReplaceAll(sanitized, "at ", "")
	sanitized = strings.ReplaceAll(sanitized, "on ", "")
	for _, layout := range DATE_TIME_LAYOUTS {
		if eventTime, err := time.Parse(layout, sanitized); err == nil {
			return eventTime, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date %s (%s)", input, sanitized)
}

const ICS_FORMAT = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//hacksw/handcal//NONSGML v1.0//EN
BEGIN:VEVENT
UID:%s-%s@jrgrover-messages.com
DTSTAMP:%s
DTSTART:%s
END:VEVENT
END:VCALENDAR`
const ICS_ISO = "20060102T150405Z"

func TimeToICS(ref time.Time) string {
	dtstamp := time.Now().UTC().Format(ICS_ISO)
	mseconds := time.Now().UTC().Format(".000000")
	dtstart := ref.Format(ICS_ISO)
	return fmt.Sprintf(ICS_FORMAT, dtstamp, mseconds, dtstamp, dtstart)
}

func ParsePListData(data []byte, rootKey string) (*plist.UID, []any, error) {
	plistDictionary := make(map[string]any, 0)
	var topValue, objectsValue, rootValue any
	var top map[string]any
	var objects []any
	var ok bool
	var rootID plist.UID

	if err := plist.NewDecoder(bytes.NewReader(data)).Decode(plistDictionary); err != nil {
		return nil, nil, fmt.Errorf("decoding plist to plistDictionary: %w", err)
	}
	if topValue, ok = plistDictionary["$top"]; !ok {
		return nil, nil, fmt.Errorf("no $top key found in plist root")
	}
	if top, ok = topValue.(map[string]any); !ok {
		return nil, nil, fmt.Errorf("could not coerce $top value in plist to map[strings]any: %T", topValue)
	}
	if objectsValue, ok = plistDictionary["$objects"]; !ok {
		return nil, nil, fmt.Errorf("no $objects key found in plist root")
	}
	if objects, ok = objectsValue.([]any); !ok {
		return nil, nil, fmt.Errorf("could not coerce $objects value in plist to []any: %T", objectsValue)
	}
	if rootValue, ok = top[rootKey]; !ok {
		return nil, nil, fmt.Errorf("could not find \"root\" in top map of plist")
	}
	if rootID, ok = rootValue.(plist.UID); !ok {
		return nil, nil, fmt.Errorf("could not coerce root to plist.UID: %T", rootValue)
	}

	return &rootID, objects, nil
}

func FlatObjectMapFromPlistData(data []byte, rootKey string) (map[string]any, error) {
	rootID, objects, err := ParsePListData(data, rootKey)
	if err != nil {
		return nil, err
	}

	return FlattenObject(objects[*rootID], "", objects), nil
}

func FlattenObject(input any, namespace string, objects []any) map[string]any {
	results := map[string]any{}

	if inputAsMapStringAny, ok := input.(map[string]any); ok {
		if len(inputAsMapStringAny) == 2 {
			if _, okClass := inputAsMapStringAny["$class"]; okClass {
				if nsObjects, okNSObjects := inputAsMapStringAny["NS.objects"]; okNSObjects {
					arrayFlattened := FlattenObject(nsObjects, namespace, objects)
					maps.Copy(results, arrayFlattened)
					return results
				}
			}
		} else if len(inputAsMapStringAny) == 3 {
			if _, okClass := inputAsMapStringAny["$class"]; okClass {
				if _, okNSBase := inputAsMapStringAny["NS.base"]; okNSBase {
					if NSrelative, okNSRelative := inputAsMapStringAny["NS.relative"]; okNSRelative {
						relativeFlattened := FlattenObject(NSrelative, namespace, objects)
						maps.Copy(results, relativeFlattened)
						return results
					}
				}
			}
		}
		for key, value := range inputAsMapStringAny {
			if key == "$class" || key == "$classname" {
				continue
			}
			valueFlattened := FlattenObject(value, fmt.Sprintf("%s/%s", namespace, key), objects)
			maps.Copy(results, valueFlattened)
		}
		return results
	}

	if inputAsArray, ok := input.([]any); ok {
		for i, item := range inputAsArray {
			itemName := fmt.Sprintf("%s/%d", namespace, i)
			itemFlattened := FlattenObject(item, itemName, objects)
			maps.Copy(results, itemFlattened)
		}
		return results
	}

	if inputAsPlistUID, ok := input.(plist.UID); ok {
		referencedObject := objects[inputAsPlistUID]
		referencedFlattened := FlattenObject(referencedObject, namespace, objects)
		maps.Copy(results, referencedFlattened)
		return results
	}

	results[namespace] = input
	return results
}

const IMAGE_URL_PREVIEW = `<div style="border-radius: 5px;">
		<img style="max-width: 350px; max-height: 200px;" src="%s"/>
		<a style="margin: 5px;" href="%s">
			<h5>%s</h5>
			<p style="font-weight: lighter;">%s</p>
		</a>
	</div>`

const ICON_TEXT_URL_PREVIEW = `<div style="border-radius: 5px; display: flex; align-items: center;">
	<a style="margin: 5px; flex: 1 1 auto;" href="%s">
		<h5>%s</h5>
		<p style="font-weight: lighter;">%s</p>
	</a>
	<img style="max-width: 35px; max-height: 35px; flex: 1 1 auto;" src="%s"/>
</div>`
const TEXT_URL_PREVIEW = `<div style="border-radius: 5px;">
	<a style="margin: 5px;" href="%s">
		<h5>%s</h5>
		<p style="font-weight: lighter;">%s</p>
	</a>
</div>`

type PListValue interface {
	string | *string | uint64 | float64 | bool | map[string]any | []byte | []any
}

func GetValueAsTypeFromMapKey[T PListValue](input map[string]any, key string) (*T, error) {
	var value any
	var asType T
	var ok bool
	if value, ok = input[key]; !ok {
		return nil, fmt.Errorf("key %s was not present in map", key)
	}
	if asType, ok = value.(T); !ok {
		return nil, fmt.Errorf("value was not of type %T: %T", new(*T), value)
	}
	return &asType, nil
}

func URLPreviewFromFlatPlistData(flatPlistData map[string]any) (result string, err error) {
	if isPlaceholderValue, err := GetValueAsTypeFromMapKey[bool](flatPlistData, "/richLinkIsPlaceholder"); err == nil && *isPlaceholderValue {
		return result, fmt.Errorf("url preview is placeholder, ignoring")
	} else if err != nil {
		return result, fmt.Errorf("getting placeholder key from plist: %w", err)
	}

	var urlString *string
	if urlString, err = GetValueAsTypeFromMapKey[string](flatPlistData, "/richLinkMetadata/URL/NS.relative"); err != nil {
		if urlString, err = GetValueAsTypeFromMapKey[string](flatPlistData, "/richLinkMetadata/originalURL/NS.relative"); err != nil {
			return result, fmt.Errorf("finding URL in preview data: %w", err)
		}
	}

	hostnameOrUrl := *urlString
	parsedUrl, err := url.Parse(*urlString)
	if err == nil {
		hostnameOrUrl = parsedUrl.Hostname()
	}

	var title *string
	if title, err = GetValueAsTypeFromMapKey[string](flatPlistData, "/richLinkMetadata/title"); err != nil {
		return result, fmt.Errorf("getting title in preview data: %w", err)
	}

	if singleImageUrl, err := GetValueAsTypeFromMapKey[string](flatPlistData, "/richLinkMetadata/imageMetadata/URL"); err != nil {
		return fmt.Sprintf(IMAGE_URL_PREVIEW, *singleImageUrl, *urlString, *title, hostnameOrUrl), nil
	}

	// TODO: Sometimes there are multiple images

	var iconUrl *string
	for iconIndex := 0; ; iconIndex += 1 {
		if nextIconUrl, err := GetValueAsTypeFromMapKey[string](flatPlistData, fmt.Sprintf("/richLinkMetadata/icons/%d/URL", iconIndex)); err != nil {
			break
		} else {
			iconUrl = nextIconUrl
		}
	}

	if iconUrl != nil {
		// TOOD: Add color to this if it exists in the plist
		return fmt.Sprintf(ICON_TEXT_URL_PREVIEW, *urlString, *title, hostnameOrUrl, *iconUrl), nil
	}

	return fmt.Sprintf(TEXT_URL_PREVIEW, *urlString, *title, hostnameOrUrl), nil
}

func SortChatHandlesIDs(unsortedChatHandlesIDs string) string {
	dbMessageChatHandleIDs := strings.Split(unsortedChatHandlesIDs, CHAT_HANDLES_IDS_SEPARATOR)
	slices.Sort(dbMessageChatHandleIDs)
	return strings.Join(dbMessageChatHandleIDs, CHAT_HANDLES_IDS_SEPARATOR)
}

func ConvertDBMessage(m DBMessage, defaultHandleID string) (*Message, error) {
	message := Message{}
	message.DBRowID = m.RowID
	message.Text = m.Text
	message.Subject = m.Subject
	message.ItemType = m.ItemType
	message.GUID = m.GUID
	message.BalloonBundleID = m.BalloonBundleID
	message.GroupActionType = m.GroupActionType
	message.NewGroupTitle = m.NewGroupTitle
	message.ReplyToGUID = m.ReplyToGUID
	message.IsFromMe = m.IsFromMe
	message.PayloadData = m.PayloadData
	message.ChatGUID = m.ChatGUID
	message.DBDate = m.Date
	message.IsSent = m.IsSent
	message.OtherID = m.OtherID
	message.HandleID = m.HandleID
	message.Attachments = m.Attachments
	message.ChatHandlesIDs = SortChatHandlesIDs(m.ChatHandlesIDs)

	if message.IsFromMe {
		message.HandleID = defaultHandleID
	}

	message.CreatedAt = time.Unix(AppleEpochUnix, m.Date)
	if m.DateRead != 0 {
		message.ReadAt = time.Unix(AppleEpochUnix, m.DateRead)
		message.IsRead = true
	}
	if m.DateEdited != 0 {
		message.EditedAt = time.Unix(AppleEpochUnix, m.DateEdited)
		message.IsEdited = true
	}
	if m.DateRetracted != 0 {
		message.RetractedAt = time.Unix(AppleEpochUnix, m.DateRetracted)
		message.IsRetracted = true
	}

	if len(m.AttributedBody) > 0 {
		if attributedString, err := DecodeStreamTypedComponents(m.AttributedBody); err != nil {
			return nil, fmt.Errorf("[%d] failed to decode attributedBody of %s: %v", m.RowID, m.GUID, err)
		} else {
			message.AttributedString = *attributedString
		}
	}
	if len(m.MessageSummaryInfo) > 0 {
		if editedMessageParts, err := EditedMessagePartsFromMessageSummaryInfo(m.MessageSummaryInfo); err != nil {
			if message.IsEdited {
				return nil, fmt.Errorf("[%d] failed to convert message_summary_info to edited message parts: %v", m.RowID, err)
			}
		} else {
			if !message.IsEdited && len(editedMessageParts) > 1 {
				return nil, fmt.Errorf("[%d] message has message_summary_info of length %d but was not edited", m.RowID, len(editedMessageParts))
			}
			message.EditedMessageParts = editedMessageParts
		}
	}

	if len(m.ThreadOriginatorPart) > 0 {
		// The thread_originator_part field seems to have three parts separated by colons.
		// The first two parts look like the part index, the third one is something else.
		// TODO this might not be reliable
		message.ReplyToPart, _ = strconv.Atoi(strings.Split(m.ThreadOriginatorPart, ":")[0])
	}
	/*
		if message.IsFromMe {
			message.Sender.LocalID = ""
		}
	*/
	if len(m.TapbackTargetGUID) > 0 {
		var err error
		message.Tapback, err = m.ParseTapback()
		if err != nil {
			return nil, fmt.Errorf("[%d] Failed to parse tapback in %s: %v", m.RowID, m.GUID, err)
		}
	}

	return &message, nil
}

func ConvertCalendarEventToHref(calendarEvent any, createdAt time.Time) (*string, error) {
	if calendarPlistBytes, ok := calendarEvent.([]byte); !ok {
		return nil, fmt.Errorf("calendar attribute could not be coerced to bytes: %f", calendarEvent)
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
							if eventTime, err := BestEffortDateTimeParse(previousObjectString, createdAt); err == nil {
								ics := TimeToICS(eventTime)
								icsBase64 := base64.URLEncoding.EncodeToString([]byte(ics))
								href := fmt.Sprintf("data:text/calendar;base64,%s", icsBase64)
								return &href, nil
							}
						}
					}
				}
			}
		}
		return nil, nil
	}
}

const (
	THUMBNAIL_SIZE      = 512
	THUMBNAIL_SIZE_HALF = THUMBNAIL_SIZE / 2
)

func AddMessagesIconToAvatarImage(avatar []byte) ([]byte, error) {
	avatarImage, _, err := image.Decode(bytes.NewReader(avatar))
	if err != nil {
		return avatar, err
	}

	messagesIconFile, err := os.Open("./img/Messages.png")
	if err != nil {
		return avatar, err
	}
	messagesIconOriginal, _, err := image.Decode(messagesIconFile)
	if err != nil {
		return avatar, err
	}

	b := avatarImage.Bounds()
	/* TODO: crop the image if it's not a square
	if b.Max.X != b.Max.Y {}
	*/

	edited := image.NewRGBA(image.Rect(0, 0, THUMBNAIL_SIZE, THUMBNAIL_SIZE))
	draw.NearestNeighbor.Scale(edited, edited.Bounds(), avatarImage, b, draw.Over, nil)

	CENTER_TO_CORNER_DISTANCE := math.Sqrt(2 * math.Pow(float64(THUMBNAIL_SIZE_HALF), 2))
	CIRCLE_TO_THUMBNAIL_EDGE := CENTER_TO_CORNER_DISTANCE - float64(THUMBNAIL_SIZE_HALF)
	CENTER_TO_ICON_CORNER_DISTANCE := int(math.Sqrt(math.Pow(CIRCLE_TO_THUMBNAIL_EDGE, 2) / 2))
	ICON_SIZE := THUMBNAIL_SIZE_HALF - (2 * CENTER_TO_ICON_CORNER_DISTANCE)
	ICON_XY := THUMBNAIL_SIZE_HALF + CENTER_TO_ICON_CORNER_DISTANCE

	if messagesIconOriginal.Bounds().Max.X > ICON_SIZE || messagesIconOriginal.Bounds().Max.Y > ICON_SIZE {
		messagesIconScaled := image.NewRGBA(image.Rect(0, 0, ICON_SIZE, ICON_SIZE))
		draw.NearestNeighbor.Scale(messagesIconScaled, messagesIconScaled.Bounds(), messagesIconOriginal, messagesIconOriginal.Bounds(), draw.Over, nil)
		draw.Draw(edited, image.Rect(ICON_XY, ICON_XY, ICON_XY+ICON_SIZE, ICON_XY+ICON_SIZE), messagesIconScaled, image.Pt(0, 0), draw.Over)
	} else {
		draw.Draw(edited, image.Rect(ICON_XY, ICON_XY, ICON_XY+ICON_SIZE, ICON_XY+ICON_SIZE), messagesIconOriginal, image.Pt(0, 0), draw.Over)
	}

	buf := new(bytes.Buffer)
	err = jpeg.Encode(buf, edited, &jpeg.Options{Quality: 90})
	if err != nil {
		return avatar, err
	}
	return buf.Bytes(), err
}
