package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/GroveJay/matrix-macOS-Messages-bridge/pkg/connector"
	"github.com/GroveJay/matrix-macOS-Messages-bridge/pkg/macos"
	"github.com/rs/zerolog"
	"go.mau.fi/zeroconfig"
	"gopkg.in/yaml.v3"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

func prepareLog(yamlConfig []byte) (*zerolog.Logger, error) {
	var cfg zeroconfig.Config
	err := yaml.Unmarshal(yamlConfig, &cfg)
	if err != nil {
		return nil, err
	}
	return cfg.Compile()
}

// This could be loaded from a file rather than hardcoded
const logConfig = `
min_level: trace
writers:
- type: stdout
  format: pretty-colored
`

func test_oascript_vcard_image() {
	contactID := ""
	vcardResult, stderr, err := macos.RunOsascript(macos.GetContactVCard, contactID)
	println("stderr:" + stderr)
	println("err:" + err.Error())
	println(fmt.Sprintf("stdout len: %d", len(vcardResult)))
	imageBytes, err := macos.GetImageFromVCard(vcardResult)
	if err != nil {
		println(err)
		return
	}
	println(fmt.Sprintf("bytes length: %d", len(imageBytes)))
	f, err := os.Create("test.jpg")
	print("error:" + err.Error())
	f.Write(imageBytes)
}

func checkError(err error) bool {
	if err != nil {
		fmt.Print(err.Error())
		return true
	}
	return false
}

func test_get_chat_details() {
	logger, err := prepareLog([]byte(logConfig))
	checkError(err)
	messagesClient, err := macos.GetMessagesClient("foobar", logger)
	checkError(err)
	contactsClient, err := macos.GetContactsClient("foobar")
	checkError(err)
	chatMap, err := messagesClient.GetAllChatIDsNames()
	checkError(err)
	contactsMap, err := contactsClient.GetContactsMap()
	checkError(err)
	for ID := range chatMap {
		chatID := macos.MakeMessagesPortalID("foobar", ID)
		println(ID)
		chatName, avatar, err := messagesClient.GetChatDetails(chatID)
		checkError(err)
		println("\tName: " + *chatName)
		if avatar != nil {
			println("\tAvatar: " + avatar.ID)
		} else {
			println("\tAvatar: nil")
		}
		memberMap, err := messagesClient.GetChatMemberMap(chatID, "foobar")
		checkError(err)

		macos.SupplementMemberMapWithContactsMap(&memberMap, contactsMap, *contactsClient)
		println("\tMembers:")
		for k, v := range memberMap {
			memberStrings := make([]string, 1)
			memberStrings = append(memberStrings, string(k))
			nickName := "Nick: "
			if v.Nickname != nil {
				nickName = nickName + *v.Nickname
			}
			memberStrings = append(memberStrings, nickName)
			name := "Name: "
			if v.UserInfo != nil && v.UserInfo.Name != nil {
				name = name + *v.UserInfo.Name
			}
			memberStrings = append(memberStrings, name)
			avatarID := "AvatarID: "
			if v.UserInfo != nil && v.UserInfo.Avatar != nil {
				avatarID = avatarID + string(v.UserInfo.Avatar.ID)
			}
			memberStrings = append(memberStrings, avatarID)
			println("\t\t" + strings.Join(memberStrings, " "))
		}
	}
}

func test_typedstream() {
	logger, err := prepareLog([]byte(logConfig))
	checkError(err)
	messagesClient, err := macos.GetMessagesClient("foobar", logger)
	checkError(err)
	messages, err := messagesClient.GetMessagesBetween(33492, 33494)
	checkError(err)
	println(fmt.Sprintf("Got %d messages", len(messages)))
	for _, message := range messages {
		println(fmt.Sprintf("[%d][Attachments: %d] [text: %s]", message.RowID, len(message.Attachments), message.Text))
	}
}

func test_parse_message_summary_info() {
	r := []byte{}
	parts, err := macos.EditedMessagePartsFromMessageSummaryInfo(r)
	checkError(err)
	for _, part := range parts {
		println(fmt.Sprintf("part: status: %d", part.Status))
	}
}

func test_parse_all_messages(dumpAttributedBodyToFiles bool) {
	logger, err := prepareLog([]byte(logConfig))
	checkError(err)
	messagesClient, err := macos.GetMessagesClient("foobar", logger)
	checkError(err)
	messages, err := messagesClient.GetMessagesNewerThan(0) // 772582691221725952
	checkError(err)
	testHandleMessages(messages, dumpAttributedBodyToFiles, logger)
}

func check_balloon_bundle_payload_data(message macos.Message) {
	if message.PayloadData != nil {
		if len(message.AttributedString.RangedAttributes) == 1 {
			onlyRangedAttribute := message.AttributedString.RangedAttributes[0]
			if _, ok := onlyRangedAttribute.AttributeMap[macos.LinkAttributeName]; ok {
				// write_bytes_to_filename(fmt.Sprintf("%d-SingleLink-PayloadData", message.RowID), message.PayloadData)
				test, err := macos.FlatObjectMapFromPlistData(message.PayloadData, "root")
				checkError(err)
				fmt.Printf("%s\n", test)
			}
			fmt.Printf("\n")
		} else {
			fmt.Printf("Too many attributed string ranges to tell?\n")
		}
	}
}

func write_bytes_to_filename(fileName string, contents []byte) {
	f, err := os.Create(fileName)
	checkError(err)
	f.Write([]byte(contents))
	f.Close()
}

func test_parse_surrounding_messages(messageID string) {
	logger, err := prepareLog([]byte(logConfig))
	checkError(err)
	messagesClient, err := macos.GetMessagesClient("foobar", logger)
	checkError(err)
	messageIDNumber, err := strconv.ParseInt(messageID, 10, 64)
	checkError(err)
	startID := messageIDNumber - 1
	endID := messageIDNumber + 1
	messages, err := messagesClient.GetMessagesBetween(int(startID), int(endID))
	checkError(err)
	for _, message := range messages {
		check_balloon_bundle_payload_data(*message)
	}
	// testHandleMessages(messages, true, logger)
}

func testHandleMessages(messages []*macos.Message, dump bool, logger *zerolog.Logger) {
	println(fmt.Sprintf("parsing %d message(s)", len(messages)))
	mc := &connector.MessagesClient{
		UserLogin: &bridgev2.UserLogin{
			UserLogin: &database.UserLogin{
				ID: networkid.UserLoginID("foobar"),
			},
			Log: *logger,
		},
		DryRun: true,
	}

	for _, message := range messages {
		if dump {
			write_bytes_to_filename(fmt.Sprintf("%d-attributedBody", message.RowID), message.AttributedBody)
		}
		mc.HandleiMessage(message)
	}
}

func test_parse_phone_number() {
	stdout := ""
	formattedPhoneNumber, err := macos.ParseFormatPhoneNumber(stdout, "US")
	checkError(err)
	println(fmt.Sprintf("got phone: %s", *formattedPhoneNumber))
}

func test_decode_stream_typed(file string) {
	attributedBody, err := os.ReadFile(file)
	checkError(err)
	m, err := macos.DecodeStreamTypedComponents(attributedBody)
	if checkError(err) {
		return
	}
	println(fmt.Sprintf("%s\n%d ranges\n%s", m.Value, len(m.RangedAttributes), m))

	/*
		parts, err := m.ConvertAttributedStringToFormattedHTMLParts()
		checkError(err)
		println(fmt.Sprintf("\n%d parts", len(parts)))
	*/
}

func main() {
	if len(os.Args) > 1 {
		args := os.Args[1:]
		firstArg := args[0]
		if strings.Contains(firstArg, "-attributedBody") {
			test_decode_stream_typed(firstArg)
		} else {
			test_parse_surrounding_messages(firstArg)
		}
	} else {
		test_parse_all_messages(false)
	}
}
