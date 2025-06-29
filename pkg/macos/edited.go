package macos

import (
	"bytes"
	"fmt"
	"strconv"

	"howett.net/plist"
)

type EditedMessageStatus int

const (
	EditedMessageStatusEdited EditedMessageStatus = iota
	EditedMessageStatusUnsent
	EditedMessageStatusOriginal
)

func (s EditedMessageStatus) String() string {
	switch s {
	case EditedMessageStatusEdited:
		return "Edited"
	case EditedMessageStatusUnsent:
		return "Unsent"
	case EditedMessageStatusOriginal:
		return "Original"
	}
	return "Unknown"
}

type EditedMessagePart struct {
	Status      EditedMessageStatus
	EditHistory []EditedEvent
}

type EditedEvent struct {
	Date             int64
	Text             *string
	AttributedString NSMutableAttributedString
	GUID             *string
}

const TIMESTAMP_FACTOR = 1000000000

type ValidTypes interface {
	map[string]any | float64 | []byte | []any | string
}

func EditedMessagePartsFromMessageSummaryInfo(messageSummaryInfo []byte) ([]*EditedMessagePart, error) {
	plistDictionary := make(map[string]any, 0)
	if err := plist.NewDecoder(bytes.NewReader(messageSummaryInfo)).Decode(plistDictionary); err != nil {
		return nil, fmt.Errorf("decoding plist to plistDictionary: %w", err)
	}
	editedMessageParts := []*EditedMessagePart{}

	otrAsMap, err := GetValueAsTypeFromMapKey[map[string]any](plistDictionary, "otr")
	if err != nil {
		return nil, err
	}

	// TODO: Why is this here?
	for range *otrAsMap {
		editedMessageParts = append(editedMessageParts, &EditedMessagePart{
			Status:      EditedMessageStatusOriginal,
			EditHistory: []EditedEvent{},
		})
	}

	if ecAsMap, err := GetValueAsTypeFromMapKey[map[string]any](plistDictionary, "ec"); err == nil {
		for k, v := range *ecAsMap {
			events, ok := v.([]any)
			if !ok {
				return nil, fmt.Errorf("casting %s in 'ec' map as array", k)
			}

			parsedKey, err := strconv.Atoi(k)
			if err != nil {
				return nil, fmt.Errorf("parsing %s as int: %w", k, err)
			}

			for i, event := range events {
				data, ok := event.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("casting event %d from key %s as map", i, k)
				}

				timestamp, err := GetValueAsTypeFromMapKey[float64](data, "d")
				if err != nil {
					return nil, fmt.Errorf("casting timestamp key 'd' to int: %w", err)
				}
				date := int64(*timestamp) * TIMESTAMP_FACTOR

				typedstreamBytes, err := GetValueAsTypeFromMapKey[[]byte](data, "t")
				if err != nil {
					return nil, fmt.Errorf("casting typedstream key 't' to []byte: %w", err)
				}

				attributedString, err := DecodeStreamTypedComponents(*typedstreamBytes)
				if err != nil {
					return nil, fmt.Errorf("getting typedstream components: %w", err)
				}
				text := attributedString.Value

				// It's ok if guid is null?
				guid, _ := GetValueAsTypeFromMapKey[string](data, "bcg")

				if parsedKey >= 0 && parsedKey < len(editedMessageParts) {
					editedMessageParts[parsedKey].Status = EditedMessageStatusEdited
					editedMessageParts[parsedKey].EditHistory = append(editedMessageParts[parsedKey].EditHistory, EditedEvent{
						Date:             date,
						Text:             &text,
						AttributedString: *attributedString,
						GUID:             guid,
					})
				}
			}
		}
	}

	if rpAsArray, err := GetValueAsTypeFromMapKey[[]any](plistDictionary, "rp"); err == nil {
		for index, unsentIndex := range *rpAsArray {
			unsentIndexUnsignedInt, ok := unsentIndex.(uint64)
			if !ok {
				return nil, fmt.Errorf("failed casting rp at index %x to uint64", index)
			}
			unsentIndexInt := int(unsentIndexUnsignedInt)
			if unsentIndexInt >= 0 && unsentIndexInt < len(editedMessageParts) {
				editedMessageParts[unsentIndexInt].Status = EditedMessageStatusUnsent
			}
		}
	}

	return editedMessageParts, nil
}
