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
	Date       int64
	Text       *string
	Components []Archivable
	GUID       *string
}

const TIMESTAMP_FACTOR = 1000000000

func GetValueAsMapFromMapKey(input map[string]any, key string) (map[string]any, error) {
	if entry, ok := input[key]; !ok {
		return nil, fmt.Errorf("no '%s' key in input map", key)
	} else {
		if entryAsMap, ok := entry.(map[string]any); !ok {
			return nil, fmt.Errorf("casting %s to map[string]any", key)
		} else {
			return entryAsMap, nil
		}
	}
}

func GetValueAsFloat64FromMapKey(input map[string]any, key string) (*float64, error) {
	if entry, ok := input[key]; !ok {
		return nil, fmt.Errorf("no '%s' key in input map", key)
	} else {
		if entryAsFloat64, ok := entry.(float64); !ok {
			return nil, fmt.Errorf("casting %s to float64", key)
		} else {
			return &entryAsFloat64, nil
		}
	}
}

func GetValueAsByteArrayFromMapKey(input map[string]any, key string) ([]byte, error) {
	if entry, ok := input[key]; !ok {
		return nil, fmt.Errorf("no '%s' key in input map", key)
	} else {
		if entryAsByteArray, ok := entry.([]byte); !ok {
			return nil, fmt.Errorf("casting %s to []byte", key)
		} else {
			return entryAsByteArray, nil
		}
	}
}

func GetValueAsArrayFromMapKey(input map[string]any, key string) ([]any, error) {
	if entry, ok := input[key]; !ok {
		return nil, fmt.Errorf("no '%s' key in input map", key)
	} else {
		if entryAsArray, ok := entry.([]any); !ok {
			return nil, fmt.Errorf("casting %s to []any", key)
		} else {
			return entryAsArray, nil
		}
	}
}

func GetValueAsStringFromMapKey(input map[string]any, key string) (*string, error) {
	if entry, ok := input[key]; !ok {
		return nil, fmt.Errorf("no '%s' key in input map", key)
	} else {
		if entryAsString, ok := entry.(string); !ok {
			return nil, fmt.Errorf("casting %s to string", key)
		} else {
			return &entryAsString, nil
		}
	}
}

func EditedMessagePartsFromMessageSummaryInfo(messageSummaryInfo []byte) ([]*EditedMessagePart, error) {
	plistDictionary := make(map[string]any, 0)
	if err := plist.NewDecoder(bytes.NewReader(messageSummaryInfo)).Decode(plistDictionary); err != nil {
		return nil, fmt.Errorf("decoding plist to plistDictionary: %w", err)
	}
	editedMessageParts := []*EditedMessagePart{}

	otrAsMap, err := GetValueAsMapFromMapKey(plistDictionary, "otr")
	if err != nil {
		return nil, err
	}

	for range otrAsMap {
		editedMessageParts = append(editedMessageParts, &EditedMessagePart{
			Status:      EditedMessageStatusOriginal,
			EditHistory: []EditedEvent{},
		})
	}

	if ecAsMap, err := GetValueAsMapFromMapKey(plistDictionary, "ec"); err == nil {
		for k, v := range ecAsMap {
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

				timestamp, err := GetValueAsFloat64FromMapKey(data, "d")
				if err != nil {
					return nil, fmt.Errorf("casting timestamp key 'd' to int: %w", err)
				}
				date := int64(*timestamp) * TIMESTAMP_FACTOR

				typedstreamBytes, err := GetValueAsByteArrayFromMapKey(data, "t")
				if err != nil {
					return nil, fmt.Errorf("casting typedstream key 't' to []byte: %w", err)
				}

				components, err := DecodeTypedStreamComponents(typedstreamBytes)
				if err != nil {
					return nil, fmt.Errorf("getting typedstream components: %w", err)
				}
				text := GetTextFromComponents(components)

				// It's ok if guid is null?
				guid, _ := GetValueAsStringFromMapKey(data, "bcg")

				if parsedKey >= 0 && parsedKey < len(editedMessageParts) {
					editedMessageParts[parsedKey].Status = EditedMessageStatusEdited
					editedMessageParts[parsedKey].EditHistory = append(editedMessageParts[parsedKey].EditHistory, EditedEvent{
						Date:       date,
						Text:       text,
						Components: components,
						GUID:       guid,
					})
				}
			}
		}
	}

	if rpAsArray, err := GetValueAsArrayFromMapKey(plistDictionary, "rp"); err == nil {
		for index, unsentIndex := range rpAsArray {
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
