package macos

type ArchivableTypes struct {
	Types []Type
}

var _ Archivable = (*ArchivableTypes)(nil)

func (a ArchivableTypes) as_nsstring() *string {
	panic("Placeholder type cannot be parsed as string")
}

func (a ArchivableTypes) as_nsnumber_float() *float64 {
	panic("Placeholder type cannot be parsed as float")
}

func (a ArchivableTypes) as_nsnumber_int() *int64 {
	panic("Placeholder type cannot be parsed as string")
}

func (a ArchivableTypes) getRange() (*int64, *uint64) {
	panic("Cannot get range from Placeholder type")
}

func (a ArchivableTypes) getDictionaryLength() int {
	panic("Cannot get dictionary length from Placeholder type")
}
