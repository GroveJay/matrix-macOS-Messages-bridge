package macos

import "fmt"

type ComponentTypeKey string

const (
	FileTransferGUIDAttributeName  ComponentTypeKey = "__kIMFileTransferGUIDAttributeName"
	AudioTranscription             ComponentTypeKey = "IMAudioTranscription"
	InlineMediaHeightAttributeName ComponentTypeKey = "__kIMInlineMediaHeightAttributeName"
	InlineMediaWidthAttributeName  ComponentTypeKey = "__kIMInlineMediaWidthAttributeName"
	FilenameAttributeName          ComponentTypeKey = "__kIMFilenameAttributeName"
	MentionConfirmedMention        ComponentTypeKey = "__kIMMentionConfirmedMention"
	LinkAttributeName              ComponentTypeKey = "__kIMLinkAttributeName"
	OneTimeCodeAttributeName       ComponentTypeKey = "__kIMOneTimeCodeAttributeName"
	CalendarEventAttributeName     ComponentTypeKey = "__kIMCalendarEventAttributeName"
	TextBoldAttributeName          ComponentTypeKey = "__kIMTextBoldAttributeName"
	TextUnderlineAttributeName     ComponentTypeKey = "__kIMTextUnderlineAttributeName"
	TextItalicAttributeName        ComponentTypeKey = "__kIMTextItalicAttributeName"
	TextStrikethroughAttributeName ComponentTypeKey = "__kIMTextStrikethroughAttributeName"
	TextEffectAttributeName        ComponentTypeKey = "__kIMTextEffectAttributeName"
)

type CombinedComponent any

type CombinedComponentAttachment struct {
	AttachmentMeta AttachmentMeta
}

type CombinedComponentText struct {
	TextRangeEffects []TextRangeEffect
}

type CombinedComponentRetraction struct{}

type CombinedComponentResult interface{}

type CombinedComponentResultNew struct {
	CombinedComponent CombinedComponent
}

type CombinedComponentResultContinuation struct {
	TextRangeEffect TextRangeEffect
}

func GetCombinedComponent(components []Archivable, text string, start int, end int, charIndexTable []int) CombinedComponentResult {
	rangeStart := 0
	rangeEnd := len(text)
	if start > 0 && start < len(charIndexTable) {
		rangeStart = charIndexTable[start]
	}
	if end > 0 && end < len(charIndexTable) {
		rangeEnd = charIndexTable[end]
	}
	for index, component := range components {
		keyName := component.as_nsstring()
		if keyName != nil {
			var keyNameAsInterface interface{} = *keyName
			keyNameAsComponentTypeKey, ok := keyNameAsInterface.(ComponentTypeKey)
			if !ok {
				continue
			}
			switch keyNameAsComponentTypeKey {
			case FileTransferGUIDAttributeName:
				attachmentMeta := GetAttachmentMetaFromComponents(components)
				if attachmentMeta == nil {
					return nil
				}
				return CombinedComponentResultNew{
					CombinedComponent: CombinedComponentAttachment{
						AttachmentMeta: *attachmentMeta,
					},
				}
			case MentionConfirmedMention:
				mentionIndex := index + 1
				if mentionIndex >= len(components) {
					return nil
				}
				mentionString := ""
				if mentionStringRef := components[mentionIndex].as_nsstring(); mentionStringRef != nil {
					mentionString = *mentionStringRef
				}

				return CombinedComponentResultContinuation{
					TextRangeEffect: TextRangeEffect{
						Start: rangeStart,
						End:   rangeEnd,
						TextEffect: TextEffectMention{
							Mention: mentionString,
						},
					},
				}
			case LinkAttributeName:
				linkIndex := index + 2
				if linkIndex >= len(components) {
					return nil
				}
				linkString := "#"
				if linkStringRef := components[linkIndex].as_nsstring(); linkStringRef != nil {
					linkString = *linkStringRef
				}

				return CombinedComponentResultContinuation{
					TextRangeEffect: TextRangeEffect{
						Start: rangeStart,
						End:   rangeEnd,
						TextEffect: TextEffectLink{
							Link: linkString,
						},
					},
				}
			case OneTimeCodeAttributeName:
				return CombinedComponentResultContinuation{
					TextRangeEffect: TextRangeEffect{
						Start:      rangeStart,
						End:        rangeEnd,
						TextEffect: TextEffectOTP{},
					},
				}
			case CalendarEventAttributeName:
				return CombinedComponentResultContinuation{
					TextRangeEffect: TextRangeEffect{
						Start: rangeStart,
						End:   rangeEnd,
						TextEffect: TextEffectConversion{
							Conversion: ConversionTypeTimezone,
						},
					},
				}
			case TextBoldAttributeName, TextUnderlineAttributeName, TextItalicAttributeName, TextStrikethroughAttributeName:
				resolvedStyles := ResolveStyles(components)
				return CombinedComponentResultContinuation{
					TextRangeEffect: TextRangeEffect{
						Start: rangeStart,
						End:   rangeEnd,
						TextEffect: TextEffectStyles{
							Styles: resolvedStyles,
						},
					},
				}
			case TextEffectAttributeName:
				effectIndex := index + 1
				if effectIndex >= len(components) {
					return nil
				}
				var animation AnimationType = 0
				if animationRef := components[effectIndex].as_nsnumber_int(); animationRef != nil {
					animation = AnimationType(*animationRef)
				}

				return CombinedComponentResultContinuation{
					TextRangeEffect: TextRangeEffect{
						Start: rangeStart,
						End:   rangeEnd,
						TextEffect: TextEffectAnimation{
							Animation: animation,
						},
					},
				}
			}
		}
	}
	return CombinedComponentResultContinuation{
		TextRangeEffect: TextRangeEffect{
			Start:      rangeStart,
			End:        rangeEnd,
			TextEffect: TextEffectDefault{},
		},
	}
}

func ConvertArchivablesToCombinedComponents(components []Archivable, componentString *string) []CombinedComponent {
	// Skip first index as it was used to get the raw string
	componentIndex := 1
	combinedComponents := []CombinedComponent{}
	// We want to index into the message text, so we need a table to align
	// Apple's indexes with the actual chars, not the bytes
	charIndexTable := []int{}
	for index := range *componentString {
		charIndexTable = append(charIndexTable, index)
	}
	currentEnd := 0
	var currentStart int
	// println(fmt.Sprintf("Going through %d components starting from %d, string: %s with length %d", len(components), componentIndex, *componentString, len(*componentString)))
	// println(fmt.Sprintf("charIndexTable of length %d", len(charIndexTable)))

	for componentIndex < len(components) {
		currentComponent := components[componentIndex]
		start, end := currentComponent.getRange()
		// println(fmt.Sprintf("Component %d", componentIndex))
		if start != nil && end != nil {
			currentStart = currentEnd
			currentEnd += int(*end)
			// println(fmt.Sprintf("Setting current start and end to %d, %d", currentStart, currentEnd))
		} else {
			// println("Skipping component")
			componentIndex += 1
			continue
		}

		componentIndex += 1
		numberAttributes := 0
		if componentIndex < len(components) {
			numberAttributes = components[componentIndex].getDictionaryLength()
		}

		if numberAttributes > 0 {
			componentIndex += 1
		}
		// println(fmt.Sprintf("Getting %d dictionary objects from component %d", numberAttributes, componentIndex))
		selectedComponents := getNDictionaryObjects(components, componentIndex, numberAttributes)
		// println(fmt.Sprintf("Converting %d selectedComponents to combined component from componentIndex %d with %d attributes, currentStart %d, currentEnd %d", len(selectedComponents), componentIndex, numberAttributes, currentStart, currentEnd ))
		if result := GetCombinedComponent(selectedComponents, *componentString, currentStart, currentEnd, charIndexTable); result != nil {
			// println(fmt.Sprintf("combined component was not nil: %d", combinedComponentResult.Variant))
			switch combinedComponentResult := result.(type) {
			case CombinedComponentResultNew:
				combinedComponents = append(combinedComponents, combinedComponentResult.CombinedComponent)
			case CombinedComponentResultContinuation:
				// println(fmt.Sprintf("Component type Continuation, current combined length: %d", len(combinedComponents)))
				textRangeEffect := combinedComponentResult.TextRangeEffect
				if len(combinedComponents) == 0 {
					combinedComponents = append(combinedComponents, CombinedComponentText{
						TextRangeEffects: []TextRangeEffect{textRangeEffect},
					})
				} else if combinedComponentText, ok := combinedComponents[len(combinedComponents)-1].(CombinedComponentText); ok {
					// println(fmt.Sprintf("Adding attributes to last component at %d", len(combinedComponents)-1))
					combinedComponentText.TextRangeEffects = append(combinedComponentText.TextRangeEffects, textRangeEffect)
				} else {
					combinedComponents = append(combinedComponents, CombinedComponentText{
						TextRangeEffects: []TextRangeEffect{textRangeEffect},
					})
				}
			default:
				panic(fmt.Sprintf("invalid type: %T", combinedComponentResult))
			}
		}
		// This feels like it has potential for an infinite loop if no components are returned?
		componentIndex += len(selectedComponents)
	}
	return combinedComponents
}
