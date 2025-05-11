package macos

type ArchivableObject struct {
	ArchivableData
	ArchivableClass
}

var _ Archivable = (*ArchivablePlaceholder)(nil)

func (a ArchivableObject) as_nsstring() *string {
	if a.Class.name != "NSString" && a.Class.name != "NSMutableString" {
		return nil
	} else {
		result := first_output_data_as_type[*string](a)
		// println("Got string for ArchivableObject: %s", &result)
		return result
	}
}

func (a ArchivableObject) as_nsnumber_float() *float64 {
	if a.Class.name != "NSNumber" {
		return nil
	} else {
		return first_output_data_as_type[*float64](a)
	}
}

func (a ArchivableObject) as_nsnumber_int() *int64 {
	if a.Class.name != "NSNumber" {
		return nil
	} else {
		return first_output_data_as_type[*int64](a)
	}
}

func (a ArchivableObject) getRange() (*int64, *uint64) {
	return nil, nil
	// panic("Cannot get range from ArchivableObject type")
}

func (a ArchivableObject) getDictionaryLength() int {
	if a.Class.name != "NSDictionary" {
		return 0
	} else {
		length := first_output_data_as_type[*int64](a)
		if length == nil {
			return 0
		}
		return int(*length) * 2
	}
}
