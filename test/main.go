package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

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
		panic(err)
	}
	return false
}

func test_get_chat_info() {
	logger, err := prepareLog([]byte(logConfig))
	checkError(err)
	mc := &connector.MessagesClient{
		UserLogin: &bridgev2.UserLogin{
			UserLogin: &database.UserLogin{
				ID: networkid.UserLoginID("foobar"),
			},
			Log: *logger,
		},
		DryRun: true,
	}
	mc.MacOSMessagesClient, err = macos.GetMessagesClient("foobar", logger)
	checkError(err)
	mc.MacOSContactsClient, err = macos.GetContactsClient("foobar")
	checkError(err)
	mc.GetChatInfo(context.TODO(), &bridgev2.Portal{
		Portal: &database.Portal{
			PortalKey: networkid.PortalKey{
				ID: "MessagesID|+19737966824|+15183205992;+18609779884;+19788950168",
			},
		},
	})
}

func test_parse_all_messages() {
	logger, err := prepareLog([]byte(logConfig))
	checkError(err)
	messagesClient, err := macos.GetMessagesClient("foobar", logger)
	checkError(err)
	messages, err := messagesClient.GetMessagesNewerThan(773912942745048064) // 0)
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

func test_parse_single_message(guid string) {
	logger, err := prepareLog([]byte(logConfig))
	checkError(err)
	messagesClient, err := macos.GetMessagesClient("foobar", logger)
	checkError(err)
	message, err := messagesClient.GetMessageByGUID(guid)
	checkError(err)
	testHandleMessages([]*macos.DBMessage{message}, logger)
}

func testHandleMessages(messages []*macos.DBMessage, logger *zerolog.Logger) {
	println(fmt.Sprintf("parsing %d message(s)", len(messages)))
	tw := tabwriter.NewWriter(os.Stdout, 1, 0, 1, ' ', tabwriter.Debug)
	fmt.Fprintln(tw, strings.Join([]string{
		"RowID",
		"Date",
		"GUID",
		"Me",
		"IT",
		"GAT",
		"Chat GUID",
		"Chat Handles IDs",
		"H.ID",
		"O.ID",
		"len(atch)",
		"len(body)",
		"BB ID",
		"TB Type",
		"TB Emoji",
		"TB Target",
	}, "\t"))
	for _, message := range messages {
		fmt.Fprintln(tw, strings.Join([]string{
			fmt.Sprintf("%d", message.RowID),
			fmt.Sprintf("%d", message.Date),
			message.GUID,
			fmt.Sprintf("%t", message.IsFromMe),
			fmt.Sprintf("%d", message.ItemType),
			fmt.Sprintf("%d", message.GroupActionType),
			message.ChatGUID,
			message.ChatHandlesIDs,
			message.HandleID,
			message.OtherID,
			fmt.Sprintf("%d", len(message.Attachments)),
			fmt.Sprintf("%d", len(message.AttributedBody)),
			message.BalloonBundleID,
			fmt.Sprintf("%d", message.TapbackType),
			message.TapbackEmoji,
			message.TapbackTargetGUID,
		}, "\t"))
	}
	tw.Flush()
	mc := &connector.MessagesClient{
		UserLogin: &bridgev2.UserLogin{
			UserLogin: &database.UserLogin{
				ID: networkid.UserLoginID("foobar"),
			},
			Log: *logger,
		},
		DryRun: true,
	}
	fmt.Fprintln(tw, strings.Join([]string{
		"RowID",
		"Date",
		"GUID",
		"Me",
		"IT",
		"GAT",
		"Chat GUID",
		"Chat Handles IDs",
		"H.ID",
		"O.ID",
		"len(atch)",
		"len(editparts)",
		"BB ID",
		"TB Type",
		"TB Emoji",
		"TB Target",
	}, "\t"))
	for _, message := range messages {
		message, err := macos.ConvertDBMessage(*message, string("CURRENT.USER"))
		if err != nil {
			println(fmt.Sprintf("ERROR converting message: %v", err))
			continue
		}
		tbType := macos.TapbackType(0)
		tbEmoji := ""
		tbTargetGUID := ""
		if message.Tapback != nil {
			tbType = message.Tapback.Type
			tbEmoji = message.Tapback.Emoji
			tbTargetGUID = message.Tapback.TargetGUID
		}
		fmt.Fprintln(tw, strings.Join([]string{
			fmt.Sprintf("%d", message.DBRowID),
			fmt.Sprintf("%d", message.DBDate),
			message.GUID,
			fmt.Sprintf("%t", message.IsFromMe),
			fmt.Sprintf("%d", message.ItemType),
			fmt.Sprintf("%d", message.GroupActionType),
			message.ChatGUID,
			message.ChatHandlesIDs,
			message.HandleID,
			message.OtherID,
			fmt.Sprintf("%d", len(message.Attachments)),
			fmt.Sprintf("%d", len(message.EditedMessageParts)),
			message.BalloonBundleID,
			fmt.Sprintf("%d", tbType),
			tbEmoji,
			tbTargetGUID,
		}, "\t"))
		mc.HandleiMessage(message)
	}
	tw.Flush()

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
			test_parse_single_message(firstArg)
			//test_parse_surrounding_messages(firstArg)
		}
	} else {
		// test_get_chat_details()
		// test_parse_all_messages()
		test_get_chat_info()
	}
}
