package macos

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nyaruka/phonenumbers"
	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
)

const FITNESS_RECEIVER = "$(kIMTranscriptPluginBreadcrumbTextReceiverIdentifier)"

var AppleEpoch = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
var AppleEpochUnix = AppleEpoch.Unix()
var AppleEpochUnixNano = AppleEpoch.UnixNano()

func MakeMessagesPortalID(userLoginID networkid.UserLoginID, chatGUID string) networkid.PortalID {
	return networkid.PortalID(fmt.Sprintf("%s:%s:%s", "MessagesID", userLoginID, chatGUID))
}

func ChatGUIDFromPortalID(portalID networkid.PortalID) string {
	parts := strings.Split(string(portalID), ":")
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
	if num, err := phonenumbers.Parse(phoneNumber, countryCode); err != nil {
		return nil, err
	} else {
		userID := networkid.UserID(phonenumbers.Format(num, phonenumbers.E164))
		return &userID, nil
	}
}

func ReplaceHomeDirectory(input string) (string, error) {
	if strings.HasPrefix(input, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		return filepath.Join(home, input[2:]), nil
	}
	return input, nil
}

// https://github.com/tinkerator/xxd/blob/main/xxd.go
func Dump(data []byte) (lines []string) {
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
		for i := 0; i < count; i++ {
			c := data[index+i]
			parts = append(parts, fmt.Sprintf("%02x", c))
			if c < 32 || c >= 127 {
				c = byte('.')
			}
			ch[1+i+offset] = c
		}
		for i := offset + count; i < 16; i++ {
			parts = append(parts, "  ")
		}
		parts = append(parts, string(ch[:1+offset+count]))
		lines = append(lines, strings.Join(parts, " "))
		index += count
		n -= count
	}
	return
}

func FilterAttachments(log *zerolog.Logger, messageAttachments []*Attachment, combinedComponents []CombinedComponent) []*Attachment {
	attachmentMap := make(map[string]*Attachment, len(messageAttachments))
	for _, messageAttachment := range messageAttachments {
		attachmentMap[messageAttachment.GUID] = messageAttachment
	}
	filteredAttachments := make([]*Attachment, 0, len(messageAttachments))
	for _, component := range combinedComponents {
		if combinedComponentAttachment, ok := component.(CombinedComponentAttachment); ok {
			if combinedComponentAttachment.AttachmentMeta.GUID != nil {
				fileGUID := *combinedComponentAttachment.AttachmentMeta.GUID
				attachment, ok := attachmentMap[fileGUID]
				if ok {
					filteredAttachments = append(filteredAttachments, attachment)
				} else {
					log.Warn().Msgf("Didn't find attachment %s in message", fileGUID)
				}
			}
		}
	}
	return filteredAttachments
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

func ConvertConvertedMessageToString(c *bridgev2.ConvertedMessage) string {
	result := fmt.Sprintf("Converted:\n%d parts:\n", len(c.Parts))
	for _, part := range c.Parts {
		result += fmt.Sprintf(" - %s\n", part.Type)
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
