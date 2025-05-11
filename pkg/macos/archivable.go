package macos

import "fmt"

type ClassResult any

type ClassResultIndex struct {
	Index int
}

type ClassResultHierarchy struct {
	ClassHierarchy []Archivable
}

type Class struct {
	name    string
	version int
}

// Not all these types are used? (*float32, *byte, *[]byte)
type OutputDataTypes interface {
	*string | *int64 | *uint64 | *float32 | *float64 | *byte | *[]byte | *Class
}

type Archivable interface {
	as_nsstring() *string
	as_nsnumber_float() *float64
	as_nsnumber_int() *int64
	getRange() (*int64, *uint64)
	getDictionaryLength() int
}

type Archivables interface {
	ArchivablePlaceholder | ArchivableData | ArchivableClass | ArchivableObject | ArchivableTypes
}

type TypeVariant int

const (
	TypeUtf8String TypeVariant = iota
	TypeEmbeddedData
	TypeObject
	TypeSignedInt
	TypeUnsignedInt
	TypeFloat
	TypeDouble
	TypeString
	TypeArray
	TypeUnknown
)

type Type struct {
	Variant      TypeVariant
	StringValue  string
	ArraySize    int
	UnknownValue byte
}

func ByteToType(input byte) Type {
	result := Type{}
	switch input {
	case 0x40:
		result.Variant = TypeObject
	case 0x2B:
		result.Variant = TypeUtf8String
	case 0x2A:
		result.Variant = TypeEmbeddedData
	case 0x66:
		result.Variant = TypeFloat
	case 0x64:
		result.Variant = TypeDouble
	case 0x63, 0x69, 0x6c, 0x71, 0x73:
		result.Variant = TypeSignedInt
	case 0x43, 0x49, 0x4c, 0x51, 0x53:
		result.Variant = TypeUnsignedInt
	default:
		result.Variant = TypeUnknown
		result.UnknownValue = input
	}
	return result
}

func first_output_data_as_type[T OutputDataTypes](archivable Archivable) (output T) {
	return output_data_index_as_type[T](archivable, 0)
}

func output_data_index_as_type[T OutputDataTypes](archivable Archivable, index int) (output T) {
	var outputData []any
	switch a := archivable.(type) {
	case ArchivableObject:
		outputData = a.OutputData
	case ArchivableData:
		outputData = a.OutputData
	default:
		panic(fmt.Sprintf("invalid type: %T", archivable))
	}
	// println(fmt.Sprintf("Getting index %d of length %d from outputData", index, len(outputData)))
	if len(outputData) < (index + 1) {
		return nil
	}
	if output, ok := outputData[index].(T); !ok {
		// println(fmt.Sprintf("Conversion to %T failed", output))
		return nil
	} else {
		return output
	}
}

func GetTextFromComponents(components []Archivable) *string {
	if len(components) > 0 {
		first := components[0]
		return first.as_nsstring()
	}
	return nil
}

func getNDictionaryObjects(components []Archivable, startIndex int, count int) []Archivable {
	if count == 0 {
		return []Archivable{}
	}
	// This loop seems buggy. We just take items till one has a range, not the amount of items asked for?
	finalIndex := startIndex + count
	for currentIndex := startIndex; currentIndex < len(components); currentIndex++ {
		currentStart, currentEnd := components[currentIndex].getRange()
		if currentStart != nil && currentEnd != nil {
			break
		}
		finalIndex = currentIndex
	}
	return components[startIndex:(finalIndex + 1)]
}

func ResolveStyles(components []Archivable) []Style {
	resolvedStyles := []Style{}
	for _, component := range components {
		keyName := component.as_nsstring()
		if keyName != nil {
			var keyNameAsInterface interface{} = *keyName
			if keyNameAsComponentTypeKey, ok := keyNameAsInterface.(ComponentTypeKey); !ok {
				continue
			} else {
				switch keyNameAsComponentTypeKey {
				case TextBoldAttributeName:
					resolvedStyles = append(resolvedStyles, StyleBold)
				case TextUnderlineAttributeName:
					resolvedStyles = append(resolvedStyles, StyleUnderline)
				case TextItalicAttributeName:
					resolvedStyles = append(resolvedStyles, StyleItalic)
				case TextStrikethroughAttributeName:
					resolvedStyles = append(resolvedStyles, StyleStrikethrough)
				}
			}
		}
	}
	return resolvedStyles
}
