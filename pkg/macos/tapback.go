package macos

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

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

func (tapback *Tapback) Parse() (*Tapback, error) {
	if tapback.Type >= 3000 && tapback.Type < 4000 {
		tapback.Type -= TapbackRemoveOffset
		tapback.Remove = true
	}
	if strings.HasPrefix(tapback.TargetGUID, "bp:") {
		tapback.TargetGUID = tapback.TargetGUID[len("bp:"):]
	} else if strings.HasPrefix(tapback.TargetGUID, "p:") {
		targetParts := strings.Split(tapback.TargetGUID[len("p:"):], "/")
		if len(targetParts) == 2 {
			var err error
			tapback.TargetPart, err = strconv.Atoi(targetParts[0])
			if err != nil {
				return nil, fmt.Errorf("%w: '%s' (%v)", ErrInvalidTapbackTargetPart, tapback.TargetGUID, err)
			}
			tapback.TargetGUID = targetParts[1]
		} else {
			return nil, fmt.Errorf("%w: '%s'", ErrUnknownNormalTapbackTarget, tapback.TargetGUID)
		}
	} else if len(tapback.TargetGUID) != 36 {
		return nil, fmt.Errorf("%w: '%s'", ErrUnknownTapbackTargetType, tapback.TargetGUID)
	}
	return tapback, nil
}
