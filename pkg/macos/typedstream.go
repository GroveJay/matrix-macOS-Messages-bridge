package macos

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

func DecodeTypedStreamComponents(encoded []byte) ([]Archivable, error) {
	typedStreamDecoder := NewTypedStreamDecoder(encoded, false)
	if components, err := typedStreamDecoder.decodeComponents(); err != nil {
		return nil, fmt.Errorf("typedStreamDecoder.DecodeComponents: %w\n%s", err, typedStreamDecoder.Dump())
	} else {
		return components, nil
	}
}

func (t Type) String() string {
	return fmt.Sprintf("Type (%d) { StringValue \"%s\", ArraySize %d, UnknownValue %x }", t.Variant, t.StringValue, t.ArraySize, t.UnknownValue)
}

const (
	// Magic array string?
	ARRAY = 0x5b
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
	// Bytes equal or greater in value than the reference tag indicate an index in the table of already-seen types
	REFERENCE_TAG = 0x92
)

type typedStreamDecoder struct {
	debug bool

	data   []byte
	index  int
	length int

	objectTable      []Archivable
	seenEmbededTypes map[int]struct{}
	typesTable       [][]Type
	placeholder      *int
}

func NewTypedStreamDecoder(encoded []byte, debug bool) typedStreamDecoder {
	return typedStreamDecoder{
		data:             encoded,
		index:            0,
		length:           len(encoded),
		objectTable:      []Archivable{},
		seenEmbededTypes: make(map[int]struct{}),
		typesTable:       [][]Type{},
		placeholder:      nil,
		debug:            debug,
	}
}

func (t *typedStreamDecoder) Dump() string {
	result := ""
	for _, line := range Dump(t.data) {
		result += line + "\n"
	}
	return result
}

func (t *typedStreamDecoder) log(indent int, format string, args ...interface{}) {
	if t.debug {
		fmt.Printf(strings.Repeat("\t", indent)+format+"\n", args...)
	}
}

func (t *typedStreamDecoder) decodeComponents() (components []Archivable, err error) {
	t.validateHeader()
	for t.index < t.length {
		t.log(0, "starting decode loop at %x", t.index)
		if currentByte, err := t.getCurrentByte(); err != nil {
			return nil, fmt.Errorf("reading current byte looking for ending: %w", err)
		} else if *currentByte == END {
			t.log(0, "skipping END byte at %x", t.index)
			t.index += 1
			continue
		}

		if foundTypes, err := t.getTypes(false); err != nil {
			return nil, fmt.Errorf("getting types: %w", err)
		} else if foundTypes == nil {
			t.log(0, "no found types")
			continue
		} else {
			t.log(0, "Reading %d found types", len(foundTypes))
			result, err := t.readTypes(foundTypes)
			if err != nil {
				return nil, fmt.Errorf("reading %d found types (%s): %w", len(foundTypes), foundTypes, err)
			}
			if result != nil {
				t.log(0, "found Archivable %T, adding to to exisitng %d components", result, len(components))
				components = append(components, result)
			}
		}
	}
	return
}

func (t *typedStreamDecoder) validateHeader() (err error) {
	version, err := t.readUnsignedInt()
	if err != nil {
		return fmt.Errorf("reading header version number: %w", err)
	}
	signature, err := t.readString()
	if err != nil {
		return fmt.Errorf("reading header signature: %w", err)
	}
	systemVersion, err := t.readSignedInt()
	if err != nil {
		return fmt.Errorf("reading header system version: %w", err)
	}
	if *version != 4 || *signature != "streamtyped" || *systemVersion != 1000 {
		return fmt.Errorf("invalid header: [version: %d, signature: %s, systemVersion: %d]", *version, *signature, *systemVersion)
	}
	return nil
}

func (t *typedStreamDecoder) readTypes(foundTypes []Type) (Archivable, error) {
	outV := make([]any, 0)
	isObj := false

	for _, foundType := range foundTypes {
		t.log(1, "Processing type %d", foundType.Variant)
		switch foundType.Variant {
		case TypeUtf8String:
			stringValue, err := t.readString()
			if err != nil {
				return nil, fmt.Errorf("error reading Utf8String: %w", err)
			}
			outV = append(outV, stringValue)
		case TypeEmbeddedData:
			return t.readEmbeddedData()
		case TypeObject:
			isObj = true
			length := len(t.objectTable)
			t.placeholder = &length
			t.log(2, "adding placeholder to object table, current length %d", len(t.objectTable))
			t.objectTable = append(t.objectTable, ArchivablePlaceholder{})
			object, err := t.readObject()
			if err != nil {
				return nil, fmt.Errorf("error reading object: %w", err)
			}
			if object != nil {
				t.log(2, "object was read non-nil, processing %T", object)
				switch archivable := object.(type) {
				case ArchivableObject:
					if len(archivable.OutputData) != 0 {
						t.placeholder = nil
						t.objectTable = t.objectTable[:len(t.objectTable)-1]
						return object, nil
					}
					outV = append(outV, archivable.OutputData...)
				case ArchivableClass:
					outV = append(outV, archivable.Class)
				case ArchivableData:
					outV = append(outV, archivable.OutputData...)
				case ArchivablePlaceholder, ArchivableTypes:
					// These cases are used internally in the objects table but should not be present in any output
				default:
					panic(fmt.Sprintf("invalid type: %T", archivable))
				}
			}

		case TypeSignedInt:
			result, err := t.readSignedInt()
			if err != nil {
				return nil, fmt.Errorf("error reading signed int: %w", err)
			}
			outV = append(outV, result)
		case TypeUnsignedInt:
			result, err := t.readUnsignedInt()
			if err != nil {
				return nil, fmt.Errorf("error reading unsigned int: %w", err)
			}
			outV = append(outV, result)
		case TypeFloat:
			result, err := t.readFloat()
			if err != nil {
				return nil, fmt.Errorf("error reading float: %w", err)
			}
			outV = append(outV, result)
		case TypeDouble:
			result, err := t.readDouble()
			if err != nil {
				return nil, fmt.Errorf("error reading double: %w", err)
			}
			outV = append(outV, result)
		case TypeUnknown:
			outV = append(outV, &foundType.UnknownValue)
		case TypeString:
			outV = append(outV, &foundType.StringValue)
		case TypeArray:
			result, err := t.readArray(foundType.ArraySize)
			if err != nil {
				return nil, fmt.Errorf("error reading array of size %d: %w", foundType.ArraySize, err)
			}
			outV = append(outV, result)
		}
	}

	if t.placeholder != nil {
		if len(outV) != 0 {
			spot := t.placeholder
			last := outV[len(outV)-1]
			// We got a class, but do not have its respective data yet
			if lastAsClass, ok := last.(*Class); ok && lastAsClass != nil {
				t.objectTable[*spot] = ArchivableObject{
					ArchivableClass: ArchivableClass{
						Class: *lastAsClass,
					},
					ArchivableData: ArchivableData{
						OutputData: make([]any, 0),
					},
				}
			} else {
				nextSpot := *spot + 1
				if nextSpot < len(t.objectTable) {
					afterPlaceholder := t.objectTable[*spot+1]
					if archivableClass, ok := afterPlaceholder.(ArchivableClass); ok {
						// The spot after the current placeholder contains the class at the top of the class heirarchy, i.e.
						// if we get a placeholder and then find a new class heirarchy, the object table holds the class chain
						// in descending order of inheritance
						t.objectTable[*spot] = ArchivableObject{
							ArchivableClass: ArchivableClass{
								Class: archivableClass.Class,
							},
							ArchivableData: ArchivableData{
								OutputData: outV,
							},
						}
						t.placeholder = nil
						return t.objectTable[*spot], nil
					}
				}
				if *spot < len(t.objectTable) {
					seenObject := t.objectTable[*spot]
					// We got some data for a class that was already seen
					if archivableObject, ok := seenObject.(ArchivableObject); ok {
						archivableObject.OutputData = append(archivableObject.OutputData, outV...)
						t.placeholder = nil
						return t.objectTable[*spot], nil
					}
				}
				// We got some data that is not part of a class, i.e. a field in the parent object for which we don't know the name
				t.objectTable[*spot] = ArchivableData{
					OutputData: outV,
				}
				return t.objectTable[*spot], nil
			}
		}
	}

	if len(outV) != 0 && !isObj {
		return &ArchivableData{OutputData: outV}, nil
	}
	return nil, nil
}

func (t *typedStreamDecoder) readObject() (Archivable, error) {
	currentByte, err := t.getCurrentByte()
	if err != nil {
		return nil, fmt.Errorf("error getting current byte reading object: %w", err)
	}
	t.log(4, "Processing start byte for object: %x", *currentByte)
	switch *currentByte {
	case START:
		t.log(4, "Start byte for object, reading class")
		readClass, err := t.readClass()
		if err != nil {
			return nil, fmt.Errorf("error reading class for object: %w", err)
		}
		switch class := readClass.(type) {
		case ClassResultIndex:
			return t.objectTable[class.Index], nil
		case ClassResultHierarchy:
			t.objectTable = append(t.objectTable, class.ClassHierarchy...)
		default:
			return nil, fmt.Errorf("invalid class type for object: %T", class)
		}
		return nil, nil
	case EMPTY:
		t.log(4, "End byte for object, returning nil?")
		t.index += 1
		return nil, nil
	default:
		t.log(4, "Reading byte as pointer")
		index, err := t.readPointer()
		if err != nil {
			return nil, fmt.Errorf("error reading pointer for object: %w", err)
		}
		t.log(4, "Assuming pointer index %d is inside object table length %d", *index, len(t.objectTable))
		return t.objectTable[*index], nil
	}
}

func (t *typedStreamDecoder) readClass() (ClassResult, error) {
	outV := make([]Archivable, 0)
	currentByte, err := t.getCurrentByte()
	if err != nil {
		return nil, fmt.Errorf("error reading current byte for class: %w", err)
	}
	t.log(5, "Processing byte %x at %x for read class", *currentByte, t.index)
	switch *currentByte {
	case START:
		for {
			headerByte, err := t.getCurrentByte()
			if err != nil {
				return nil, fmt.Errorf("error reading current byte for header of class: %w", err)
			}
			if *headerByte == START {
				t.log(5, "Skipping header byte at %x", t.index)
				t.index += 1
				continue
			}
			break
		}

		length, err := t.readUnsignedInt()
		if err != nil {
			return nil, fmt.Errorf("error reading unsigned int for class: %w", err)
		}
		t.log(5, "Read %d as length of class name up to byte %x", *length, t.index)

		if *length >= REFERENCE_TAG {
			t.log(5, "Class name length was index, returning ClassResultIndex")
			index := *length - REFERENCE_TAG
			return ClassResultIndex{
				Index: int(index),
			}, nil
		}

		className, err := t.readNBytesAsString(int(*length))
		if err != nil {
			return nil, fmt.Errorf("error reading %d bytes as string for class name: %w", *length, err)
		}
		t.log(5, "Read %s as class name up to byte %x", *className, t.index)

		version, err := t.readUnsignedInt()
		if err != nil {
			return nil, fmt.Errorf("error reading version for class: %w", err)
		}
		t.log(5, "Read %d as class version up to byte %x", *version, t.index)
		t.typesTable = append(t.typesTable, []Type{{
			Variant:     TypeString,
			StringValue: *className,
		}})
		outV = append(outV, ArchivableClass{
			Class: Class{
				name:    *className,
				version: int(*version),
			},
		})

		readClass, err := t.readClass()
		if err != nil {
			return nil, fmt.Errorf("error reading class: %w", err)
		}
		if readClassAsHiearchy, ok := readClass.(ClassResultHierarchy); ok {
			outV = append(outV, readClassAsHiearchy.ClassHierarchy...)
		}
	case EMPTY:
		t.log(5, "Skipping empty byte at %x", t.index)
		t.index += 1
	default:
		t.log(5, "Reading pointer at index %x", t.index)
		index, err := t.readPointer()
		if err != nil {
			return nil, fmt.Errorf("error reading pointer for class: %w", err)
		}
		t.log(5, "Returning ClassResultIndex from pointer index %d", *index)
		return ClassResultIndex{
			Index: *index,
		}, nil
	}

	t.log(5, "Returning ClassResultHierarchy with %d elements", len(outV))
	return ClassResultHierarchy{
		ClassHierarchy: outV,
	}, nil
}

func (t *typedStreamDecoder) readEmbeddedData() (Archivable, error) {
	t.index += 1
	t.log(2, "Reading embedded data at %x", t.index)
	types, err := t.getTypes(true)
	if err != nil {
		return nil, fmt.Errorf("error getting types for reading embedded data: %w", err)
	}
	if len(types) != 0 {
		return t.readTypes(types)
	}
	// This is too spooky
	return nil, nil
}

func (t *typedStreamDecoder) getTypes(embeded bool) ([]Type, error) {
	firstByte, err := t.getCurrentByte()
	if err != nil {
		return nil, fmt.Errorf("retrieving first byte for type: %w", err)
	}
	t.log(1, "checking first byte %x for type at %x", *firstByte, t.index)
	switch *firstByte {
	case START:
		t.log(1, "Start byte, beginning to read a type")
		t.index += 1
		componentTypes, err := t.readType()
		if err != nil {
			return nil, fmt.Errorf("reading type after start: %w", err)
		}
		// Embedded data is stored as a C String in the objects table
		if embeded {
			t.log(1, "Embedded, adding type to object table")
			t.objectTable = append(t.objectTable, ArchivableTypes{
				Types: componentTypes,
			})
			seenEmbededTypeIndex := max(0, len(t.objectTable)-1)
			t.seenEmbededTypes[seenEmbededTypeIndex] = struct{}{}
		}
		t.log(1, fmt.Sprintf("Adding %d types to typesTable", len(componentTypes)))
		t.typesTable = append(t.typesTable, componentTypes)
		latestTypes := t.typesTable[len(t.typesTable)-1]
		return latestTypes, nil
	case END:
		t.log(1, "End byte, returning")
		return nil, nil
	default:
		t.log(1, "Other byte, skipping any identical bytes")
		for {
			currentByte, err := t.getCurrentByte()
			if err != nil {
				return nil, fmt.Errorf("reading current byte in type: %w", err)
			}
			nextByte, err := t.getNextByte()
			if err != nil {
				return nil, fmt.Errorf("reading next byte in type: %w", err)
			}
			if *currentByte == *nextByte {
				t.log(2, "Skipping identical byte %x at %x and %x", *currentByte, t.index, t.index+1)
				t.index += 1
				continue
			}
			break
		}

		t.log(1, "Reading current byte at %x as pointer", t.index)
		refTag, err := t.readPointer()
		if err != nil {
			return nil, fmt.Errorf("reading pointer for type after first byte %x: %w", *firstByte, err)
		}
		var typesFromTable []Type
		typesFromTable = nil
		typesTableLength := len(t.typesTable)
		t.log(1, "Getting types from table if ref %d is inside length %d", *refTag, typesTableLength)
		if *refTag < typesTableLength {
			typesFromTable = t.typesTable[*refTag]
		}

		if embeded {
			t.log(2, "Embedded call")
			if typesFromTable != nil {
				// We only want to include the first embedded reference tag, not subsequent references to the same embed
				t.log(2, "Types for ref exist, seeing if we should add ref ref %d to object table and seen tags", *refTag)
				if _, ok := t.seenEmbededTypes[*refTag]; !ok {
					t.objectTable = append(t.objectTable, ArchivableTypes{
						Types: typesFromTable,
					})
					t.seenEmbededTypes[*refTag] = struct{}{}
					t.log(2, "Added ref %d to object table and seen tags", *refTag)
				}
			}
		}
		return typesFromTable, nil
	}
}

func isASCIIDigit(input byte) bool {
	return input <= 57 && input >= 48
}

func byteToDigit(input byte) uint8 {
	return input - 48
}

func (t *typedStreamDecoder) readPointer() (*int, error) {
	pointer, err := t.getCurrentByte()
	if err != nil {
		return nil, fmt.Errorf("getting current byte for pointer: %w", err)
	}
	if *pointer < REFERENCE_TAG {
		return nil, fmt.Errorf("pointer (%x) was less than reference tag (%x) at index %x", *pointer, REFERENCE_TAG, t.index)
	}
	offsetPointer := int(*pointer - REFERENCE_TAG)
	t.index += 1
	return &offsetPointer, nil
}

func (t *typedStreamDecoder) readType() ([]Type, error) {
	t.log(2, "Reading length for type at %x", t.index)
	length, err := t.readUnsignedInt()
	if err != nil {
		return nil, fmt.Errorf("error reading length for type: %w", err)
	}
	t.log(2, "Read length %d for type", *length)

	typesBytes, err := t.readNBytes(int(*length))
	if err != nil {
		return nil, fmt.Errorf("error reading %d bytes for type at %x: %w", *length, t.index, err)
	}

	resultTypes := make([]Type, 0)
	if typesBytes[0] == ARRAY {
		t.log(2, "Type Bytes starts as array")
		restTypes := typesBytes[1:]
		array_length := 0
		for i := 0; i < len(restTypes) && isASCIIDigit(restTypes[i]); i++ {
			array_length = (array_length * 10) + int(byteToDigit(restTypes[i]))
		}
		if array_length == 0 {
			return nil, fmt.Errorf("zero length array found reading bytes for type: %s", string(restTypes))
		}
		t.log(2, "Got %d array length", array_length)
		resultTypes = append(resultTypes, Type{Variant: TypeArray, ArraySize: array_length})
	} else {
		for _, typeByte := range typesBytes {
			byteType := ByteToType(typeByte)
			t.log(3, "Converted %x to type %d", typeByte, byteType.Variant)
			resultTypes = append(resultTypes, byteType)
		}
	}
	return resultTypes, nil
}

func (t *typedStreamDecoder) readArray(size int) ([]byte, error) {
	return t.readNBytes(size)
}

func (t *typedStreamDecoder) readString() (*string, error) {
	length, err := t.readUnsignedInt()
	if err != nil {
		return nil, fmt.Errorf("error reading length of string: %w", err)
	}
	return t.readNBytesAsString(int(*length))
}

func (t *typedStreamDecoder) readNBytesAsString(n int) (*string, error) {
	bytesToRead, err := t.readNBytes(n)
	if err != nil {
		return nil, fmt.Errorf("error reading %d bytes for string: %w", n, err)
	}
	readString := string(bytesToRead)
	return &readString, nil
}

func (t *typedStreamDecoder) readUnsignedInt() (*uint64, error) {
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

func (t *typedStreamDecoder) readSignedInt() (*int64, error) {
	firstByte, err := t.getCurrentByte()
	if err != nil {
		return nil, fmt.Errorf("error reading first byte for signed int: %w", err)
	}
	t.index += 1
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
		if *firstByte > REFERENCE_TAG {
			if nextByte, err := t.getCurrentByte(); err != nil {
				return nil, fmt.Errorf("error reading following byte for signed int of seen type: %w", err)
			} else if *nextByte != END {
				return t.readSignedInt()
			}
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

func (t *typedStreamDecoder) readFloat() (*float32, error) {
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
	signedInt, err := t.readSignedInt()
	if err != nil {
		return nil, fmt.Errorf("erorr reading signed int for float: %w", err)
	}
	signedIntAsFloat := float32(*signedInt)
	return &signedIntAsFloat, nil
}

func (t *typedStreamDecoder) readDouble() (*float64, error) {
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
	signedInt, err := t.readSignedInt()
	if err != nil {
		return nil, fmt.Errorf("erorr default reading signed int for float: %w", err)
	}
	signedIntAsFloat := float64(*signedInt)
	return &signedIntAsFloat, nil
}

func (t *typedStreamDecoder) getNextByte() (*byte, error) {
	nextByteIndex := t.index + 1
	if nextByteIndex < t.length {
		return &(t.data[nextByteIndex]), nil
	}
	return nil, fmt.Errorf("next byte index %d out of range of encoded bytes (%d)", nextByteIndex, t.length)
}

func (t *typedStreamDecoder) readNBytes(n int) ([]byte, error) {
	startIndex := t.index
	endIndex := t.index + n
	if endIndex < t.length {
		t.index += n
		return t.data[startIndex:endIndex], nil
	}
	return nil, fmt.Errorf("end index %d out of range of encoded bytes (%d)", endIndex, t.length)
}

func (t *typedStreamDecoder) getCurrentByte() (*byte, error) {
	if t.index < t.length {
		return &(t.data[t.index]), nil
	}
	return nil, fmt.Errorf("index %d out of range of encoded bytes (%d)", t.index, t.length)
}
