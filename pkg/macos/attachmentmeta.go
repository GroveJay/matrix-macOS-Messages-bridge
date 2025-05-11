package macos

type AttachmentMeta struct {
	GUID          *string
	Transcription *string
	Height        *float64
	Width         *float64
	Name          *string
}

func GetAttachmentMetaFromComponents(components []Archivable) *AttachmentMeta {
	attachmentMeta := AttachmentMeta{}
	for index, component := range components {
		keyName := component.as_nsstring()
		if keyName != nil {
			var keyNameAsInterface any = *keyName
			if keyNameAsComponentTypeKey, ok := keyNameAsInterface.(ComponentTypeKey); !ok {
				continue
			} else {
				nextIndex := index + 1
				if nextIndex >= len(components) {
					return nil
				}
				nextComponent := components[nextIndex]
				switch keyNameAsComponentTypeKey {
				case FileTransferGUIDAttributeName:
					attachmentMeta.GUID = nextComponent.as_nsstring()
				case AudioTranscription:
					attachmentMeta.Transcription = nextComponent.as_nsstring()
				case InlineMediaHeightAttributeName:
					attachmentMeta.Height = nextComponent.as_nsnumber_float()
				case InlineMediaWidthAttributeName:
					attachmentMeta.Width = nextComponent.as_nsnumber_float()
				case FilenameAttributeName:
					attachmentMeta.Name = nextComponent.as_nsstring()
				}
			}
		}
	}
	return &attachmentMeta
}
