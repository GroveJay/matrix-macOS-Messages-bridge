package macos

import (
	"context"
	"os"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type MockMatrixAPI struct{}

// CreateRoom implements bridgev2.MatrixAPI.
func (m *MockMatrixAPI) CreateRoom(ctx context.Context, req *mautrix.ReqCreateRoom) (id.RoomID, error) {
	panic("unimplemented")
}

// DeleteRoom implements bridgev2.MatrixAPI.
func (m *MockMatrixAPI) DeleteRoom(ctx context.Context, roomID id.RoomID, puppetsOnly bool) error {
	panic("unimplemented")
}

// DownloadMedia implements bridgev2.MatrixAPI.
func (m *MockMatrixAPI) DownloadMedia(ctx context.Context, uri id.ContentURIString, file *event.EncryptedFileInfo) ([]byte, error) {
	panic("unimplemented")
}

// DownloadMediaToFile implements bridgev2.MatrixAPI.
func (m *MockMatrixAPI) DownloadMediaToFile(ctx context.Context, uri id.ContentURIString, file *event.EncryptedFileInfo, writable bool, callback func(*os.File) error) error {
	panic("unimplemented")
}

// EnsureInvited implements bridgev2.MatrixAPI.
func (m *MockMatrixAPI) EnsureInvited(ctx context.Context, roomID id.RoomID, userID id.UserID) error {
	panic("unimplemented")
}

// EnsureJoined implements bridgev2.MatrixAPI.
func (m *MockMatrixAPI) EnsureJoined(ctx context.Context, roomID id.RoomID) error {
	panic("unimplemented")
}

// GetMXID implements bridgev2.MatrixAPI.
func (m *MockMatrixAPI) GetMXID() id.UserID {
	panic("unimplemented")
}

// IsDoublePuppet implements bridgev2.MatrixAPI.
func (m *MockMatrixAPI) IsDoublePuppet() bool {
	panic("unimplemented")
}

// MarkRead implements bridgev2.MatrixAPI.
func (m *MockMatrixAPI) MarkRead(ctx context.Context, roomID id.RoomID, eventID id.EventID, ts time.Time) error {
	panic("unimplemented")
}

// MarkTyping implements bridgev2.MatrixAPI.
func (m *MockMatrixAPI) MarkTyping(ctx context.Context, roomID id.RoomID, typingType bridgev2.TypingType, timeout time.Duration) error {
	panic("unimplemented")
}

// MarkUnread implements bridgev2.MatrixAPI.
func (m *MockMatrixAPI) MarkUnread(ctx context.Context, roomID id.RoomID, unread bool) error {
	panic("unimplemented")
}

// MuteRoom implements bridgev2.MatrixAPI.
func (m *MockMatrixAPI) MuteRoom(ctx context.Context, roomID id.RoomID, until time.Time) error {
	panic("unimplemented")
}

// SendMessage implements bridgev2.MatrixAPI.
func (m *MockMatrixAPI) SendMessage(ctx context.Context, roomID id.RoomID, eventType event.Type, content *event.Content, extra *bridgev2.MatrixSendExtra) (*mautrix.RespSendEvent, error) {
	panic("unimplemented")
}

// SendState implements bridgev2.MatrixAPI.
func (m *MockMatrixAPI) SendState(ctx context.Context, roomID id.RoomID, eventType event.Type, stateKey string, content *event.Content, ts time.Time) (*mautrix.RespSendEvent, error) {
	panic("unimplemented")
}

// SetAvatarURL implements bridgev2.MatrixAPI.
func (m *MockMatrixAPI) SetAvatarURL(ctx context.Context, avatarURL id.ContentURIString) error {
	panic("unimplemented")
}

// SetDisplayName implements bridgev2.MatrixAPI.
func (m *MockMatrixAPI) SetDisplayName(ctx context.Context, name string) error {
	panic("unimplemented")
}

// SetExtraProfileMeta implements bridgev2.MatrixAPI.
func (m *MockMatrixAPI) SetExtraProfileMeta(ctx context.Context, data any) error {
	panic("unimplemented")
}

// TagRoom implements bridgev2.MatrixAPI.
func (m *MockMatrixAPI) TagRoom(ctx context.Context, roomID id.RoomID, tag event.RoomTag, isTagged bool) error {
	panic("unimplemented")
}

// UploadMedia implements bridgev2.MatrixAPI.
func (m *MockMatrixAPI) UploadMedia(ctx context.Context, roomID id.RoomID, data []byte, fileName string, mimeType string) (url id.ContentURIString, file *event.EncryptedFileInfo, err error) {
	panic("unimplemented")
}

// UploadMediaStream implements bridgev2.MatrixAPI.
func (m *MockMatrixAPI) UploadMediaStream(ctx context.Context, roomID id.RoomID, size int64, requireFile bool, cb bridgev2.FileStreamCallback) (url id.ContentURIString, file *event.EncryptedFileInfo, err error) {
	panic("unimplemented")
}

var _ bridgev2.MatrixAPI = (*MockMatrixAPI)(nil)
