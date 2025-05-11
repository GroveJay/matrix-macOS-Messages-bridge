package macos

type ArchivablePlaceholder struct{}

var _ Archivable = (*ArchivablePlaceholder)(nil)

func (a ArchivablePlaceholder) as_nsstring() *string {
	panic("Placeholder type cannot be parsed as string")
}

func (a ArchivablePlaceholder) as_nsnumber_float() *float64 {
	panic("Placeholder type cannot be parsed as float")
}

func (a ArchivablePlaceholder) as_nsnumber_int() *int64 {
	panic("Placeholder type cannot be parsed as string")
}

func (a ArchivablePlaceholder) getRange() (*int64, *uint64) {
	panic("Cannot get range from Placeholder type")
}

func (a ArchivablePlaceholder) getDictionaryLength() int {
	panic("Cannot get dictionary length from Placeholder type")
}
