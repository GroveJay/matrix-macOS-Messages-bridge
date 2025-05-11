package main

import (
	"fmt"
	"os"
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

func checkError(err error) {
	if err != nil {
		// println("Error: " + err.Error())
		panic(err)
	}
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
		chatID := networkid.PortalID(ID)
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
			if v.UserInfo.Name != nil {
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

func test_parse_attributed_body() {
	r := []byte{}
	components, err := macos.DecodeTypedStreamComponents(r)
	checkError(err)
	println(fmt.Sprintf("Got %d components", len(components)))
}

func test_parse_all_messages() {
	logger, err := prepareLog([]byte(logConfig))
	checkError(err)
	messagesClient, err := macos.GetMessagesClient("foobar", logger)
	checkError(err)
	// messages, err := messagesClient.GetMessagesBetween(33490, 33499)
	messages, err := messagesClient.GetMessagesNewerThan(0)
	checkError(err)
	println(fmt.Sprintf("parsed %d messages", len(messages)))

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
		mc.HandleiMessage(message)
	}
}

func main() {
	test_parse_all_messages()
}
