package macos

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type DBMessage struct {
	RowID         int
	Date          int64
	DateRead      int64
	DateEdited    int64
	DateRetracted int64

	IsSent         bool
	IsFromMe       bool
	IsDelivered    bool
	IsEmote        bool
	IsAudioMessage bool

	GUID                 string
	Subject              string
	Text                 string
	ChatGUID             string
	ReplyToGUID          string
	ThreadID             string
	NewGroupTitle        string
	BalloonBundleID      string
	ThreadOriginatorPart string
	HandleID             string
	OtherID              string

	GroupActionType GroupActionType
	ItemType        ItemType

	TapbackType       TapbackType
	TapbackTargetGUID string
	TapbackEmoji      string

	AttributedBody     []byte
	PayloadData        []byte
	MessageSummaryInfo []byte

	Attachments map[string]*Attachment
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

func (m *DBMessage) ParseTapback() (*Tapback, error) {
	var tapback Tapback
	if m.TapbackType >= 3000 && m.TapbackType < 4000 {
		tapback.Type = m.TapbackType - TapbackRemoveOffset
		tapback.Remove = true
	}
	if strings.HasPrefix(m.TapbackTargetGUID, "bp:") {
		tapback.TargetGUID = m.TapbackTargetGUID[len("bp:"):]
	} else if strings.HasPrefix(m.TapbackTargetGUID, "p:") {
		targetParts := strings.Split(m.TapbackTargetGUID[len("p:"):], "/")
		if len(targetParts) == 2 {
			var err error
			tapback.TargetPart, err = strconv.Atoi(targetParts[0])
			if err != nil {
				return nil, fmt.Errorf("%w: '%s' (%v)", ErrInvalidTapbackTargetPart, m.TapbackTargetGUID, err)
			}
			tapback.TargetGUID = targetParts[1]
		} else {
			return nil, fmt.Errorf("%w: '%s'", ErrUnknownNormalTapbackTarget, m.TapbackTargetGUID)
		}
	} else if len(m.TapbackTargetGUID) != 36 {
		return nil, fmt.Errorf("%w: '%s'", ErrUnknownTapbackTargetType, m.TapbackTargetGUID)
	} else {
		tapback.TargetGUID = m.TapbackTargetGUID
	}
	return &tapback, nil
}
