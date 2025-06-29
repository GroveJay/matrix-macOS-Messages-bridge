package macos

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	// Magic array string?
	ARRAY = 0x5b

	FIRST_TAG = 0x80
	// Indicates an [`i16`] in the byte stream
	I_16 = 0x81
	// Indicates an [`i32`] in the byte stream
	I_32 = 0x82
	// Indicates an [`f32`] or [`f64`] in the byte stream; the [`Type`] determines the size
	DECIMAL = 0x83
	// Indicates the start of a new object
	START = 0x84
	// Indicates that there is no more data to parse, for example the end of a class inheritance chain
	EMPTY = 0x85
	// Indicates the last byte of an object
	END = 0x86

	LAST_TAG = 0x91
	// Bytes equal or greater in value than the reference tag indicate an index in the table of already-seen types
	REFERENCE_TAG = 0x92
)

type TypeVariant int

const (
	TypeIgnored TypeVariant = iota
	TypeClass
	TypeAtom
	TypeEmbeddedData
	TypeUtf8String
	TypeSelector
	TypeObject
	TypeBoolean
	TypeUnsignedInt
	TypeSignedInt
	TypeFloat
	TypeDouble
	TypeArray
	TypeUnknown
)

type NSObject struct {
	Classes []int
	Values  []any
}

type ClassString struct {
	name    string
	version int
}

type CString struct {
	value string
}

type ComponentTypeKey string

// Other likely keys include:
/*
__kIMAnimatedEmojiAttributeName
__kIMBackgroundColorAttributeName
__kIMBreadcrumbTextOptionFlags
__kIMCallMonitorCallStatusChanged
__kIMCurrentPreferredLanguageChangedNotification
__kIMDataDetectorResultAttributeName
__kIMFileBookmarkAttributeName
__kIMFontFamilyAttributeName
__kIMFontSizeAttributeName
__kIMForegroundColorAttributeName
__kIMLockdownDeviceActivatedChangedNotification
__kIMMessageBackgroundColorAttributeName
__kIMMessageForegroundColorAttributeName
__kIMMobileNetworkManagerAirplaneModeChangedNotification
__kIMMobileNetworkManagerDataStatusChangedNotification
__kIMMyNameAttributeName
__kIMNetworkChangedNotification
__kIMNetworkChangedNotificationNetworkAvailableKey
__kIMPluginPayloadAttributeName
__kIMPreferredAccountForServiceChangedNotificationInternal
__kIMPreformattedAttributeName
__kIMReferencedHandleAttributeName
__kIMRemoteObjectsCodingClassKey
__kIMRemoteURLConnectionErrorDomain
__kIMSearchTermAttributeName
__kIMSmileyDescriptionAttributeName
__kIMSmileyLengthAttributeName
__kIMSmileySpeechDescriptionAttributeName
__kIMUniqueSmileyNumberAttributeName
*/
const (
	BaseWritingDirectionAttributeName ComponentTypeKey = "__kIMBaseWritingDirectionAttributeName"
	FileTransferGUIDAttributeName     ComponentTypeKey = "__kIMFileTransferGUIDAttributeName"
	AudioTranscription                ComponentTypeKey = "IMAudioTranscription"
	InlineMediaHeightAttributeName    ComponentTypeKey = "__kIMInlineMediaHeightAttributeName"
	InlineMediaWidthAttributeName     ComponentTypeKey = "__kIMInlineMediaWidthAttributeName"
	FilenameAttributeName             ComponentTypeKey = "__kIMFilenameAttributeName"
	MentionConfirmedMention           ComponentTypeKey = "__kIMMentionConfirmedMention"
	LinkAttributeName                 ComponentTypeKey = "__kIMLinkAttributeName"
	OneTimeCodeAttributeName          ComponentTypeKey = "__kIMOneTimeCodeAttributeName"
	CalendarEventAttributeName        ComponentTypeKey = "__kIMCalendarEventAttributeName"
	TextBoldAttributeName             ComponentTypeKey = "__kIMTextBoldAttributeName"
	TextUnderlineAttributeName        ComponentTypeKey = "__kIMTextUnderlineAttributeName"
	TextItalicAttributeName           ComponentTypeKey = "__kIMTextItalicAttributeName"
	TextStrikethroughAttributeName    ComponentTypeKey = "__kIMTextStrikethroughAttributeName"
	TextEffectAttributeName           ComponentTypeKey = "__kIMTextEffectAttributeName"
	MessagePartAttributeName          ComponentTypeKey = "__kIMMessagePartAttributeName"
	DataDetectedAttributeName         ComponentTypeKey = "__kIMDataDetectedAttributeName"
	PhoneNumberAttributeName          ComponentTypeKey = "__kIMPhoneNumberAttributeName"
	MoneyAttributeName                ComponentTypeKey = "__kIMMoneyAttributeName"
	AddressAttributeName              ComponentTypeKey = "__kIMAddressAttributeName"
	PhotoSharingAttributeName         ComponentTypeKey = "__kIMPhotoSharingAttributeName"
	LinkIsRichLinkAttributeName       ComponentTypeKey = "__kIMLinkIsRichLinkAttributeName"
	BreadcrumbTextMarkerAttributeName ComponentTypeKey = "__kIMBreadcrumbTextMarkerAttributeName"
	URLToTransferMapAttributeName     ComponentTypeKey = "__kIMUrlToTransferMapAttributeName"
	RichCardsAttributeName            ComponentTypeKey = "__kIMRichCardsAttributeName"
)

func (c *ComponentTypeKey) IsKnown() bool {
	switch *c {
	case BaseWritingDirectionAttributeName,
		FileTransferGUIDAttributeName,
		AudioTranscription,
		InlineMediaHeightAttributeName,
		InlineMediaWidthAttributeName,
		FilenameAttributeName,
		MentionConfirmedMention,
		LinkAttributeName,
		OneTimeCodeAttributeName,
		CalendarEventAttributeName,
		TextBoldAttributeName,
		TextUnderlineAttributeName,
		TextItalicAttributeName,
		TextStrikethroughAttributeName,
		TextEffectAttributeName,
		MessagePartAttributeName,
		DataDetectedAttributeName,
		PhoneNumberAttributeName,
		MoneyAttributeName,
		AddressAttributeName,
		PhotoSharingAttributeName,
		LinkIsRichLinkAttributeName:
		return true
	}
	return false
}

type RangedAttribute struct {
	cacheKey     int64
	start        int
	length       int
	AttributeMap map[ComponentTypeKey]any
}

type NSMutableAttributedString struct {
	Value            string
	RangedAttributes []RangedAttribute
}

func (n NSMutableAttributedString) String() string {
	results := []string{}

	if len(n.Value) > 0 {
		results = append(results, fmt.Sprintf("AttributedBodyString(%d):", len(n.Value)))
		if len(n.Value) < 5 || strings.Contains(n.Value, "￼") || utf8.RuneCountInString(n.Value) < 10 {
			results = append(results, strings.Join(Dump([]byte(n.Value), make([]byte, len(n.Value))), "\n"))
		} else {
			results = append(results, n.Value)
		}
	}

	results = append(results, fmt.Sprintf("Attribute Ranges: %d", len(n.RangedAttributes)))
	for i, rangedAttribute := range n.RangedAttributes {
		results = append(results, fmt.Sprintf("\t%d (%d) - %d -> %d", i, rangedAttribute.cacheKey, rangedAttribute.start, rangedAttribute.start+rangedAttribute.length))
		for k, v := range rangedAttribute.AttributeMap {
			valueString := ""
			switch value := v.(type) {
			case *int64:
				valueString = strconv.FormatInt(*value, 10)
			case *int32:
				valueString = strconv.FormatInt(int64(*value), 10)
			case *int16:
				valueString = strconv.FormatInt(int64(*value), 10)
			case *int8:
				valueString = strconv.FormatInt(int64(*value), 10)
			case *int:
				valueString = strconv.FormatInt(int64(*value), 10)
			case *string:
				valueString = *value
			case []byte:
				valueString = fmt.Sprintf("[]byte(%d)", len(value))
			default:
				valueString = fmt.Sprintf("%f", value)
			}

			results = append(results, fmt.Sprintf("\t\t%s : %s", k, valueString))
		}
	}

	return strings.Join(results, "\n")
}

func DecodeStreamTypedComponents(encoded []byte) (*NSMutableAttributedString, error) {
	streamTypedDecoder := NewStreamTypedDecoder(encoded, true)
	if attributedString, err := streamTypedDecoder.decode(); err != nil {
		return nil, fmt.Errorf("streamTypedDecoder.decode: %w\n\x1B[0m%s", err, streamTypedDecoder.Dump())
	} else {
		// fmt.Print(streamTypedDecoder.Dump())
		return attributedString, nil
	}
}

type streamTypedDecoder struct {
	debug bool

	colors []byte

	encoded []byte
	index   int
	length  int

	objectTable []any
	stringTable []string
}

func NewStreamTypedDecoder(encoded []byte, debug bool) streamTypedDecoder {
	streamTypedDecoder := streamTypedDecoder{
		encoded:     encoded,
		index:       0,
		length:      len(encoded),
		colors:      make([]byte, len(encoded)),
		objectTable: []any{},
		stringTable: []string{},
		debug:       debug,
	}
	if debug {
		streamTypedDecoder.Dump()
	}
	return streamTypedDecoder
}

func (t *streamTypedDecoder) Dump() string {
	result := ""
	for _, line := range Dump(t.encoded, t.colors) {
		result += line + "\n"
	}
	return result
}

func (t *streamTypedDecoder) decode() (*NSMutableAttributedString, error) {
	t.validateHeader()

	allContents, err := t.readAll()
	if err != nil {
		return nil, err
	}
	if len(allContents) == 0 {
		return nil, fmt.Errorf("no contents found in encoded bytes")
	}
	contents := allContents[0]
	if len(contents) == 0 {
		return nil, fmt.Errorf("no contents found in first object")
	}
	rootObject, ok := contents[0].(*NSObject)
	if !ok {
		return nil, fmt.Errorf("unable to cast root object (%f) as NSObject", contents[0])
	}
	rangedAttributes := []RangedAttribute{}
	start := 0
	if firstObject, ok := rootObject.Values[0].(*NSObject); !ok {
		return nil, fmt.Errorf("first object in contents was not NSObject")
	} else if stringValue, ok := firstObject.Values[0].(*string); !ok {
		return nil, fmt.Errorf("first NSObject was not a string")
	} else {
		values := rootObject.Values
		attributeCache := map[int64]map[ComponentTypeKey]any{}
		for i := 1; i < len(values); {
			cacheKeyIndex := i
			cacheKeyValue := values[cacheKeyIndex]
			i += 1
			cacheKey, ok := cacheKeyValue.(*int64)
			if !ok {
				return nil, fmt.Errorf("index value at %d was not a int64: %f", cacheKeyIndex, cacheKeyValue)
			}

			lengthValueIndex := i
			i += 1
			if lengthValueIndex >= len(values) {
				break
			}
			lengthValue := values[lengthValueIndex]
			length, ok := lengthValue.(*uint64)
			if !ok {
				return nil, fmt.Errorf("length value at %d was not an uint64: %f", lengthValueIndex, lengthValue)
			}

			if cachedAttributes, ok := attributeCache[*cacheKey]; ok {
				rangedAttributes = append(rangedAttributes, RangedAttribute{
					cacheKey:     *cacheKey,
					start:        start,
					length:       int(*length),
					AttributeMap: cachedAttributes,
				})
				continue
			}

			dictionaryValueIndex := i
			i += 1
			// This should never happen now?
			if dictionaryValueIndex >= len(values) {
				return nil, fmt.Errorf("tried to index out of the values looking for Dictionary for range")
			}
			dictionaryValue := values[dictionaryValueIndex]
			attributesDictionaryObject, ok := dictionaryValue.(*NSObject)
			if !ok {
				return nil, fmt.Errorf("dictionary value was not an NSObject: %f", dictionaryValue)
			}

			dictionaryValues := attributesDictionaryObject.Values
			dictionarySize, ok := dictionaryValues[0].(*int64)
			if !ok {
				return nil, fmt.Errorf("size of NSDictionary was not an int: %f", dictionaryValues[0])
			}
			attributes := make(map[ComponentTypeKey]any, *dictionarySize)
			for j := 1; j < len(dictionaryValues); {
				keyObject, ok := dictionaryValues[j].(*NSObject)
				if !ok {
					return nil, fmt.Errorf("key at %d was not an NSObject in NSDictionary: %f", j, dictionaryValues[j])
				}
				key, err := t.parseNSObject(*keyObject)
				if err != nil {
					return nil, fmt.Errorf("parsing NSObject: %w", err)
				}
				keyString, ok := key.(*string)
				if !ok {
					return nil, fmt.Errorf("key at %d was not a string: %f", j, key)
				}
				componentTypeKey := ComponentTypeKey(*keyString)
				valueObject, ok := dictionaryValues[j+1].(*NSObject)
				if !ok {
					return nil, fmt.Errorf("value at %d was not an NSObject in NSDictionary: %f", j, dictionaryValues[j+1])
				}
				value, err := t.parseNSObject(*valueObject)
				if err != nil {
					return nil, fmt.Errorf("parsing value object at %d: %w", j+1, err)
				}
				attributes[componentTypeKey] = value
				j += 2
			}
			rangedAttributes = append(rangedAttributes, RangedAttribute{
				cacheKey:     *cacheKey,
				start:        start,
				length:       int(*length),
				AttributeMap: attributes,
			})
			attributeCache[*cacheKey] = attributes
			start += int(*length)
		}
		return &NSMutableAttributedString{
			Value:            *stringValue,
			RangedAttributes: rangedAttributes,
		}, nil
	}
}

func (t *streamTypedDecoder) parseNSObject(object NSObject) (any, error) {
	firstClass := t.objectTable[object.Classes[0]]
	firstClassString, ok := firstClass.(ClassString)
	if !ok {
		return nil, fmt.Errorf("first class did not reference a Class object: %f", firstClass)
	}

	switch firstClassString.name {
	case "NSNumber":
		typeValueIndex := 0
		valueValueIndex := 1
		typeValue := object.Values[typeValueIndex]
		valueValue := object.Values[valueValueIndex]
		if typeValueString, ok := typeValue.(*string); !ok {
			return nil, fmt.Errorf("unable to coerce first NSNumber object into encoding string: %f", typeValue)
		} else {
			typeVariant := StringToTypeVariant(*typeValueString)

			switch typeVariant {
			case TypeUnsignedInt:
				if result, ok := valueValue.(*uint64); !ok {
					return nil, fmt.Errorf("unable to coerce value into uint64: %f", valueValue)
				} else {
					return result, nil
				}
			case TypeSignedInt:
				if result, ok := valueValue.(*int64); !ok {
					return nil, fmt.Errorf("unable to coerce value into int64: %f", valueValue)
				} else {
					return result, nil
				}
			case TypeFloat:
				if result, ok := valueValue.(*float32); !ok {
					return nil, fmt.Errorf("unable to coerce value into float32: %f", valueValue)
				} else {
					return result, nil
				}
			case TypeDouble:
				if result, ok := valueValue.(*float64); !ok {
					return nil, fmt.Errorf("unable to coerce value into float64: %f", valueValue)
				} else {
					return result, nil
				}
			default:
				return nil, fmt.Errorf("unrecognized encoding for NSNumber: (%s)", *typeValueString)
			}
		}
	case "NSString":
		firstObject := object.Values[len(object.Values)-1]
		if resultString, ok := firstObject.(*string); !ok {
			return nil, fmt.Errorf("unable to coerce first NSString object into string: %f", firstObject)
		} else {
			return resultString, nil
		}
	case "NSURL":
		urlValue := object.Values[1]
		if urlObject, ok := urlValue.(*NSObject); !ok {
			return nil, fmt.Errorf("unable to coerce index 1 value of NSUrl to NSObject: %f", urlValue)
		} else {
			return t.parseNSObject(*urlObject)
		}
	case "NSData", "NSMutableData":
		dataValue := object.Values[1]
		if resultValue, ok := dataValue.([]uint8); !ok {
			return nil, fmt.Errorf("unable to coerce index 1 value of NSData to []uint8: %f", dataValue)
		} else {
			return resultValue, nil
		}
	case "NSMutableDictionary", "NSDictionary":
		lengthValue := object.Values[0]
		if length, ok := lengthValue.(*int64); !ok {
			return nil, fmt.Errorf("unable to coerce %s length value to int64: %f", firstClassString.name, lengthValue)
		} else {
			result := make(map[string]any, *length)
			for i := 0; i < int(*length); i += 1 {
				keyIndex := (i * 2) + 1
				keyObject := object.Values[keyIndex]
				keyNSObject, ok := keyObject.(*NSObject)
				if !ok {
					return nil, fmt.Errorf("key object for %d key-value in %s was not NSObject: %f", keyIndex, firstClassString.name, keyObject)
				}
				keyValue, err := t.parseNSObject(*keyNSObject)
				if err != nil {
					return nil, fmt.Errorf("unable to parse key for %d key-value in %s: %w", keyIndex, firstClassString.name, err)
				}
				key, ok := keyValue.(*string)
				if !ok {
					return nil, fmt.Errorf("key was not string for %d key-value in %s: %f", keyIndex, firstClassString.name, keyValue)
				}

				valueIndex := keyIndex + 1
				valueObject := object.Values[valueIndex]
				valueNSObject, ok := valueObject.(*NSObject)
				if !ok {
					return nil, fmt.Errorf("value object for %d key-value in %s was not NSObject: %f", i, firstClassString.name, keyObject)
				}
				valueValue, err := t.parseNSObject(*valueNSObject)
				if err != nil {
					return nil, fmt.Errorf("unable to parse value for %d key-value in %s: %w", i, firstClassString.name, err)
				}
				result[*key] = valueValue
			}
			return result, nil
		}
	case "NSArray":
		lengthValue := object.Values[0]
		if length, ok := lengthValue.(*int64); !ok {
			return nil, fmt.Errorf("unable to coerce NSArray length value to int64: %f", lengthValue)
		} else {
			result := make([]any, *length)
			for i := range *length {
				arrayValue := object.Values[i+1]
				arrayNSObject, ok := arrayValue.(*NSObject)
				if !ok {
					return nil, fmt.Errorf("unable to coerce NSArray item %d into NSObject: %f", i, arrayValue)
				}
				arrayItem, err := t.parseNSObject(*arrayNSObject)
				if err != nil {
					return nil, fmt.Errorf("parsing array item: %w", err)
				}
				result[i] = arrayItem
			}
			return result, nil
		}
	}
	return nil, fmt.Errorf("unable to parse NSObject with first class name: %s", firstClassString.name)
}

func (t *streamTypedDecoder) readAll() (contents [][]any, err error) {
	for t.index < t.length {
		startIndex := t.index
		if values, err := t.readTypedValues(nil); err != nil {
			return nil, fmt.Errorf("reading typed value from %x: %w", startIndex, err)
		} else {
			contents = append(contents, values)
		}
	}
	return contents, nil
}

func (t *streamTypedDecoder) readTypedValues(head *byte) (contents []any, err error) {
	head, err = t.maybeReadByte(head)
	if err != nil {
		return nil, fmt.Errorf("reading head byte (%b): %w", *head, err)
	}
	encodingString, err := t.readSharedString(head)
	if err != nil {
		return nil, fmt.Errorf("reading encoding string: %w", err)
	}
	if encodingString == nil {
		return nil, fmt.Errorf("encoding string was empty")
	}

	encodings := parseEncodings(t.stringTable[*encodingString])
	for _, encoding := range encodings {
		value, err := t.readValueWithEncoding(encoding)
		if err != nil {
			return nil, fmt.Errorf("reading value with encoding %x: %w", encoding, err)
		}
		contents = append(contents, value)
	}
	return contents, nil
}

func parseEncodings(encodingString string) (encodings []string) {
	start := 0
	end := 1
	array := 0x0
	for end <= len(encodingString) {
		if array == '[' && encodingString[end] == ']' {
			encodings = append(encodings, encodingString[start:end+1])
			array = 0x0
			start = end + 1
			end = end + 2
			continue
		}
		if encodingString[start] == '[' {
			array = '['
			end += 1
			continue
		}
		if array != 0x0 {
			continue
		}
		encodings = append(encodings, encodingString[start:end])
		start = end
		end = end + 1
	}
	return
}

func (t *streamTypedDecoder) readValueWithEncoding(encoding string) (any, error) {
	encodingType := StringToTypeVariant(encoding)
	if encodingType == TypeUnknown {
		return nil, fmt.Errorf("type byte was not recognized")
	}
	switch encodingType {
	case TypeBoolean:
		if value, err := t.maybeReadByte(nil); err != nil {
			return nil, fmt.Errorf("reading byte for boolean: %w", err)
		} else {
			if *value == 0 {
				return false, nil
			}
			if *value == 1 {
				return true, nil
			}
			return nil, fmt.Errorf("value was not 0 or 1 for boolean (%x)", *value)
		}
	case TypeUnsignedInt:
		return t.readUnsignedInt()
	case TypeSignedInt:
		return t.readSignedInt(nil)
	case TypeFloat:
		return t.readFloat()
	case TypeDouble:
		return t.readDouble()
	case TypeEmbeddedData:
		return t.readCString()
	case TypeAtom:
		return nil, fmt.Errorf("unsupported type: Atom?")
	case TypeSelector:
		return t.readSharedString(nil)
	case TypeUtf8String:
		return t.readString()
	case TypeClass:
		return t.readClass()
	case TypeObject:
		return t.readObject()
	case TypeIgnored:
		return nil, fmt.Errorf("unsupported type: Ignored?")
	case TypeArray:
		if encoding[0] == '[' {
			if encoding[len(encoding)-1] != ']' {
				return nil, fmt.Errorf("array encoding did not end with ]: %s", encoding)
			}
			arrayLengthString := encoding[1 : len(encoding)-1]
			arrayLength := 0
			for i := 0; i < len(arrayLengthString) && isASCIIDigit(arrayLengthString[i]); i++ {
				arrayLength = (arrayLength * 10) + int(byteToDigit(arrayLengthString[i]))
			}
			if arrayLength == 0 {
				return nil, fmt.Errorf("zero length array found reading encoding: %s", encoding)
			}
			typeEncoding := arrayLengthString[len(arrayLengthString)-1]
			if typeEncoding == 'C' || typeEncoding == 'c' {
				return t.readNBytes(arrayLength)
			} else {
				// results := []any{}
				// for _ := range(length) {
				// 		results = append(results, _read_value_with_encoding(typeEncoding))
				// }
				// return results
				return nil, fmt.Errorf("unsupported array type encoding %b from encoding %s", typeEncoding, encoding)
			}
		} else if encoding[0] == '{' {
			// results := []any{}
			// name, field_type_encodings := parse_struct_encoding(typeEncoding)
			// for field_type_encoding := range(field_type_encodings) {}
			// 		results = append(results, _read_value_with_encoding(field_type_encoding))
			// return results
			return nil, fmt.Errorf("unsupported field encoding %s", encoding)
		} else {
			return nil, fmt.Errorf("unsupported array encoding %s", encoding)
		}
	default:
		return nil, fmt.Errorf("invalid encoding type %s", encoding)
	}
}

func (t *streamTypedDecoder) readObject() (result *NSObject, err error) {
	object := NSObject{
		Classes: []int{},
		Values:  []any{},
	}
	objectIndex := len(t.objectTable)
	t.objectTable = append(t.objectTable, nil)
	head, err := t.maybeReadByte(nil)
	if err != nil {
		return nil, fmt.Errorf("reading head for object: %w", err)
	}
	if *head == EMPTY {
		if t.debug {
			t.colors[t.index-1] = 31
		}
		object.Classes = append(object.Classes, -1)
	} else if *head == START {
		if t.debug {
			t.colors[t.index-1] = 105
		}
		classesRead, err := t.readClass()
		if err != nil {
			return nil, fmt.Errorf("reading classes for object: %w", err)
		}
		object.Classes = append(object.Classes, classesRead...)
		nextHead, err := t.maybeReadByte(nil)
		if err != nil {
			return nil, fmt.Errorf("reading next head for object: %w", err)
		}
		for *nextHead != END {
			typesRead, err := t.readTypedValues(nextHead)
			if err != nil {
				return nil, fmt.Errorf("reading types for object: %w", err)
			}
			object.Values = append(object.Values, typesRead...)
			nextHead, err = t.maybeReadByte(nil)
			if err != nil {
				return nil, fmt.Errorf("reading next head for object: %w", err)
			}
		}
		if t.debug {
			t.colors[t.index-1] = 103
		}
	} else {
		if t.debug {
			t.colors[t.index-1] = 95
		}
		referenceIndex := *head - REFERENCE_TAG
		reference := t.objectTable[referenceIndex]
		if referencedObject, ok := reference.(NSObject); !ok {
			return nil, fmt.Errorf("referenced object was not an NSObject: %f", reference)
		} else {
			// objects that are references to other objects do not end up in the object table
			t.objectTable = t.objectTable[:len(t.objectTable)-1]
			return &referencedObject, nil
		}

	}
	t.objectTable[objectIndex] = object
	return &object, nil
}

func (t *streamTypedDecoder) readClass() (result []int, err error) {
	head, err := t.maybeReadByte(nil)
	if err != nil {
		return nil, fmt.Errorf("error reading head byte for class: %w", err)
	}
	classes := []int{}
	for *head == START {
		if t.debug {
			t.colors[t.index-1] = 105
		}
		if name, err := t.readSharedString(nil); err != nil {
			return nil, fmt.Errorf("error reading class name class: %w", err)
		} else if name == nil {
			return nil, fmt.Errorf("class name was nil")
		} else {
			if version, err := t.readSignedInt(nil); err != nil {
				return nil, fmt.Errorf("error reading class version: %w", err)
			} else {
				if t.debug {
					t.colors[t.index-1] = 92
				}
				class := ClassString{
					name:    t.stringTable[*name],
					version: int(*version),
				}
				classes = append(classes, len(t.objectTable))
				t.objectTable = append(t.objectTable, class)
				head, err = t.maybeReadByte(nil)
				if err != nil {
					return nil, fmt.Errorf("error reading head byte for class: %w", err)
				}
			}
		}
	}
	if *head == EMPTY {
		if t.debug {
			t.colors[t.index-1] = 93
		}
		classes = append(classes, -1)
	} else {
		if t.debug {
			t.colors[t.index-1] = 35
		}
		classes = append(classes, int(*head-REFERENCE_TAG))
	}
	return classes, nil
}

func (t *streamTypedDecoder) readCString() (result *string, err error) {
	head, err := t.maybeReadByte(nil)
	if err != nil {
		return nil, fmt.Errorf("reading head byte: %w", err)
	}
	if *head == EMPTY {
		return nil, nil
	}
	if *head == START {
		if t.debug {
			t.colors[t.index-1] = 105
		}
		if stringIndex, err := t.readSharedString(nil); err != nil {
			return nil, fmt.Errorf("reading shared string: %w", err)
		} else if stringIndex == nil {
			return nil, fmt.Errorf("string index was nil for C string")
		} else if strings.Contains(t.stringTable[*stringIndex], string(rune(0x00))) {
			return nil, fmt.Errorf("zero byte not allowed in C string")
		} else {
			t.objectTable = append(t.objectTable, CString{
				value: t.stringTable[*stringIndex],
			})
			return &t.stringTable[*stringIndex], nil
		}
	}

	// t.index += 1
	objectIndex := int(*head - REFERENCE_TAG)
	if objectIndex >= len(t.objectTable) {
		return nil, fmt.Errorf("string index %d was out of range of the object table (%d)", objectIndex, len(t.objectTable)-1)
	}
	cStringObject := t.objectTable[objectIndex]
	if cString, ok := cStringObject.(CString); !ok {
		return nil, fmt.Errorf("object at index %d was not a CString: %f", objectIndex, cStringObject)
	} else {
		return &cString.value, nil
	}
}

func (t *streamTypedDecoder) readSharedString(head *byte) (result *int, err error) {
	head, err = t.maybeReadByte(head)
	if err != nil {
		return nil, fmt.Errorf("reading head byte (%b): %w", *head, err)
	}
	if *head == EMPTY {
		return nil, nil
	}
	if *head == START {
		if t.debug {
			t.colors[t.index-1] = 105
		}
		if readString, err := t.readString(); err != nil {
			return nil, fmt.Errorf("reading string failed")
		} else if readString == nil {
			return nil, fmt.Errorf("string was nil")
		} else {
			stringIndex := len(t.stringTable)
			t.stringTable = append(t.stringTable, *readString)
			return &stringIndex, nil
		}
	}

	if t.debug {
		t.colors[t.index-1] = 34
	}
	stringIndex := int(*head - REFERENCE_TAG)
	return &stringIndex, nil
}

func (t *streamTypedDecoder) validateHeader() (err error) {
	version, err := t.readUnsignedInt()
	if err != nil {
		return fmt.Errorf("reading header version number: %w", err)
	}
	signature, err := t.readString()
	if err != nil {
		return fmt.Errorf("reading header signature: %w", err)
	}
	systemVersion, err := t.readSignedInt(nil)
	if err != nil {
		return fmt.Errorf("reading header system version: %w", err)
	}
	if *version != 4 || *signature != "streamtyped" || *systemVersion != 1000 {
		return fmt.Errorf("invalid header: [version: %d, signature: %s, systemVersion: %d]", *version, *signature, *systemVersion)
	}
	if t.debug {
		for i := range t.index {
			t.colors[i] = 100
		}
	}
	return nil
}

func (t *streamTypedDecoder) readSignedInt(head *byte) (*int64, error) {
	firstByte, err := t.maybeReadByte(head)
	if err != nil {
		return nil, fmt.Errorf("error reading first byte for signed int: %w", err)
	}
	// t.index += 1
	var bytesToRead []byte
	var result int64
	switch *firstByte {
	case I_16:
		bytesToRead, err = t.readNBytes(2)
		if err != nil {
			return nil, fmt.Errorf("error reading 2 bytes for signed int: %w", err)
		}
		var resultInt16 int16
		buf := bytes.NewReader(bytesToRead)
		err := binary.Read(buf, binary.LittleEndian, &resultInt16)
		if err != nil {
			return nil, fmt.Errorf("binary.Read failed for bytes %x: %w", bytesToRead, err)
		}
		result = int64(resultInt16)
	case I_32:
		bytesToRead, err = t.readNBytes(4)
		if err != nil {
			return nil, fmt.Errorf("error reading 4 bytes for signed int: %w", err)
		}
		var resultInt32 int32
		buf := bytes.NewReader(bytesToRead)
		err := binary.Read(buf, binary.LittleEndian, &resultInt32)
		if err != nil {
			return nil, fmt.Errorf("binary.Read failed for bytes %x: %w", bytesToRead, err)
		}
		result = int64(resultInt32)
	default:
		if *firstByte >= FIRST_TAG && *firstByte <= LAST_TAG {
			return nil, fmt.Errorf("error reading signed int, invalid header byte %x", *firstByte)
		}

		bytesToRead = []byte{*firstByte}
		var resultInt8 int8
		buf := bytes.NewReader(bytesToRead)
		err := binary.Read(buf, binary.LittleEndian, &resultInt8)
		if err != nil {
			return nil, fmt.Errorf("binary.Read failed for bytes %x: %w", bytesToRead, err)
		}
		result = int64(resultInt8)
	}

	return &result, nil
}

func (t *streamTypedDecoder) readUnsignedInt() (*uint64, error) {
	firstByte, err := t.getCurrentByte()
	if err != nil {
		return nil, fmt.Errorf("error reading first byte for unsigned int: %w", err)
	}
	t.index += 1
	var bytesToRead []byte
	var result uint64
	switch *firstByte {
	case I_16:
		bytesToRead, err = t.readNBytes(2)
		if err != nil {
			return nil, fmt.Errorf("error reading two bytes for unsigned int: %w", err)
		}
		result = uint64(binary.LittleEndian.Uint16(bytesToRead))
	case I_32:
		bytesToRead, err = t.readNBytes(4)
		if err != nil {
			return nil, fmt.Errorf("error reading two bytes for unsigned int: %w", err)
		}
		result = uint64(binary.LittleEndian.Uint32(bytesToRead))
	default:
		result = uint64(uint8(*firstByte))
	}

	return &result, nil
}

func (t *streamTypedDecoder) readFloat() (*float32, error) {
	typeByte, err := t.getCurrentByte()
	if err != nil {
		return nil, fmt.Errorf("error reading byte for float: %w", err)
	}
	switch *typeByte {
	case DECIMAL:
		t.index += 1
		bytesToRead, err := t.readNBytes(4)
		if err != nil {
			return nil, fmt.Errorf("error reading bytes for float: %w", err)
		}
		floatResult := math.Float32frombits(binary.LittleEndian.Uint32(bytesToRead))
		return &floatResult, nil
	case I_16, I_32:
		// I thought we were reading a float...
	default:
		t.index += 1
	}
	signedInt, err := t.readSignedInt(nil)
	if err != nil {
		return nil, fmt.Errorf("erorr reading signed int for float: %w", err)
	}
	signedIntAsFloat := float32(*signedInt)
	return &signedIntAsFloat, nil
}

func (t *streamTypedDecoder) readDouble() (*float64, error) {
	typeByte, err := t.getCurrentByte()
	if err != nil {
		return nil, fmt.Errorf("error reading byte for float: %w", err)
	}
	switch *typeByte {
	case DECIMAL:
		t.index += 1
		bytesToRead, err := t.readNBytes(8)
		if err != nil {
			return nil, fmt.Errorf("error reading bytes for float: %w", err)
		}
		floatResult := math.Float64frombits(binary.LittleEndian.Uint64(bytesToRead))
		return &floatResult, nil
	case I_16, I_32:
		// I thought we were reading a float...
	default:
		t.index += 1
	}
	signedInt, err := t.readSignedInt(nil)
	if err != nil {
		return nil, fmt.Errorf("erorr default reading signed int for float: %w", err)
	}
	signedIntAsFloat := float64(*signedInt)
	return &signedIntAsFloat, nil
}

func (t *streamTypedDecoder) readString() (*string, error) {
	length, err := t.readUnsignedInt()
	if err != nil {
		return nil, fmt.Errorf("error reading length of string: %w", err)
	}
	if t.debug {
		t.colors[t.index-1] = 44
		for i := t.index; i < t.index+int(*length); i++ {
			t.colors[i] = 32
		}
	}
	return t.readNBytesAsString(int(*length))
}

func (t *streamTypedDecoder) readNBytesAsString(n int) (*string, error) {
	bytesToRead, err := t.readNBytes(n)
	if err != nil {
		return nil, fmt.Errorf("error reading %d bytes for string: %w", n, err)
	}
	readString := string(bytesToRead)
	return &readString, nil
}

func (t *streamTypedDecoder) maybeReadByte(previouslyReadByte *byte) (*byte, error) {
	if previouslyReadByte != nil {
		return previouslyReadByte, nil
	}
	if readBytes, err := t.readNBytes(1); err != nil {
		return nil, fmt.Errorf("reading 1 byte: %w", err)
	} else if len(readBytes) != 1 {
		return nil, fmt.Errorf("read %d bytes when trying to read 1", len(readBytes))
	} else {
		return &readBytes[0], nil
	}
}

func (t *streamTypedDecoder) readNBytes(n int) ([]byte, error) {
	startIndex := t.index
	endIndex := t.index + n
	if endIndex <= t.length {
		t.index += n
		return t.encoded[startIndex:endIndex], nil
	}
	return nil, fmt.Errorf("end index %d out of range of encoded bytes (%d)", endIndex, t.length)
}

func (t *streamTypedDecoder) getCurrentByte() (*byte, error) {
	if t.index < t.length {
		return &(t.encoded[t.index]), nil
	}
	return nil, fmt.Errorf("index %d (%x) out of range of encoded bytes (%d)", t.index, t.index, t.length)
}

func StringToTypeVariant(encodingString string) TypeVariant {
	if len(encodingString) > 1 {
		return TypeArray
	}
	encoding := byte(encodingString[0])
	switch encoding {
	case 0x21:
		return TypeIgnored
	case 0x23:
		return TypeClass
	case 0x25:
		return TypeAtom
	case 0x2A:
		return TypeEmbeddedData
	case 0x2B:
		return TypeUtf8String
	case 0x3A:
		return TypeSelector
	case 0x40:
		return TypeObject
	case 0x42:
		return TypeBoolean
	case 0x43, 0x49, 0x4c, 0x51, 0x53:
		return TypeUnsignedInt
	case 0x63, 0x69, 0x6c, 0x71, 0x73:
		return TypeSignedInt
	case 0x66:
		return TypeFloat
	case 0x64:
		return TypeDouble
	default:
		return TypeUnknown
	}
}
