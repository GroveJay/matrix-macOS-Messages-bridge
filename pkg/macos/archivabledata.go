package macos

type ArchivableData struct {
	OutputData []any
}

var _ Archivable = (*ArchivableData)(nil)

func (a ArchivableData) as_nsstring() *string {
	return nil
	// panic("Data type cannot be parsed as string")
}

func (a ArchivableData) as_nsnumber_float() *float64 {
	panic("Data type cannot be parsed as float")
}

func (a ArchivableData) as_nsnumber_int() *int64 {
	panic("Data type cannot be parsed as string")
}

func (a ArchivableData) getRange() (*int64, *uint64) {
	if len(a.OutputData) != 2 {
		return nil, nil
	}
	rangeStart := first_output_data_as_type[*int64](a)
	if rangeStart == nil {
		return nil, nil
	}
	rangeEnd := output_data_index_as_type[*uint64](a, 1)
	if rangeEnd == nil {
		return nil, nil
	}
	return rangeStart, rangeEnd
}

func (a ArchivableData) getDictionaryLength() int {
	return 0
	// panic("Cannot get dictionary length from Data type")
}
