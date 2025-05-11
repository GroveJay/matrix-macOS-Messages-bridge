package macos

type ArchivableClass struct {
	Class Class
}

var _ Archivable = (*ArchivableClass)(nil)

func (a ArchivableClass) as_nsstring() *string {
	panic("Class type cannot be parsed as string")
}

func (a ArchivableClass) as_nsnumber_float() *float64 {
	panic("Class type cannot be parsed as float")
}

func (a ArchivableClass) as_nsnumber_int() *int64 {
	panic("Class type cannot be parsed as string")
}

func (a ArchivableClass) getRange() (*int64, *uint64) {
	panic("Cannot get range from Class type")
}

func (a ArchivableClass) getDictionaryLength() int {
	panic("Cannot get dictionary length from Class type")
}
