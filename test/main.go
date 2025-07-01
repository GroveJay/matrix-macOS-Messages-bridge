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

func test_parse_all_messages() {
	logger, err := prepareLog([]byte(logConfig))
	checkError(err)
	messagesClient, err := macos.GetMessagesClient("foobar", logger)
	checkError(err)
	messages, err := messagesClient.GetMessagesNewerThan(0) // 772941069808000128
	checkError(err)
	testHandleMessages(messages, logger)
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
	testHandleMessages(messages, logger)
}

func testHandleMessages(messages []*macos.DBMessage, logger *zerolog.Logger) {
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
		message, err := macos.ConvertDBMessage(*message, string("CURRENT.USER"))
		if err != nil {
			println(fmt.Sprintf("ERROR converting message: %v", err))
			continue
		}
		mc.HandleiMessage(message)
	}
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
		test_parse_all_messages()
	}
}
