package macos

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

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
	return os.ReadFile(a.PathOnDisk)
}

func (a *Attachment) GetMimeType() string {
	if a.MimeType == "" {
		mime, err := mimetype.DetectFile(a.PathOnDisk)
		if err != nil {
			return ""
		}
		a.MimeType = mime.String()
	}
	return a.MimeType
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
		// TODO: Get the height and width from the actual image instead of the metadata that may not be present
		if attachmentMeta.Height != nil {
			convertedMessagePart.Content.Info.Height = int(*attachmentMeta.Height)
		}
		if attachmentMeta.Width != nil {
			convertedMessagePart.Content.Info.Width = int(*attachmentMeta.Width)
		}
		convertedMessagePart.Content.MsgType = event.MsgImage
	case strings.HasPrefix(mimeType, "video"):
		convertedMessagePart.Content.MsgType = event.MsgVideo
	case strings.HasPrefix(mimeType, "audio"):
		convertedMessagePart.Content.MsgType = event.MsgAudio
		if attachmentMeta.Transcription != nil && len(*attachmentMeta.Transcription) != 0 {
			convertedMessagePart.Content.Body += fmt.Sprintf(" | Transcript: %s", *attachmentMeta.Transcription)
		}
	default:
		convertedMessagePart.Content.MsgType = event.MsgFile
	}
	return convertedMessagePart
}
