package macos

import (
	"fmt"
	"html/template"
)

type Style int

const (
	StyleBold Style = iota
	StyleItalic
	StyleStrikethrough
	StyleUnderline
)

type AnimationType int64

const (
	Big     AnimationType = 5
	Small   AnimationType = 11
	Shake   AnimationType = 9
	Nod     AnimationType = 8
	Explode AnimationType = 12
	Ripple  AnimationType = 4
	Bloom   AnimationType = 6
	Jitter  AnimationType = 10
	Unknown AnimationType = 0
)

type ConversionType int

const (
	ConversionTypeCurrency ConversionType = iota
	ConversionTypeDistance
	ConversionTypeTemperature
	ConversionTypeTimezone
	ConversionTypeVolume
	ConversionTypeWeight
)

type TextEffectMention struct {
	Mention string
}

type TextEffectLink struct {
	Link string
}

type TextEffectStyles struct {
	Styles []Style
}

type TextEffectAnimation struct {
	Animation AnimationType
}

type TextEffectConversion struct {
	Conversion ConversionType
}

type TextEffectOTP struct{}

type TextEffectDefault struct{}

type TextEffect any

type TextEffects interface {
	TextEffectMention | TextEffectLink | TextEffectStyles | TextEffectAnimation | TextEffectConversion | TextEffectOTP | TextEffectDefault
}

type TextRangeEffect struct {
	Start      int
	End        int
	TextEffect TextEffect
}

func FormatTextRangeEffectsOnText(text string, textRangeEffects []TextRangeEffect) string {
	formattedText := ""
	for _, textRangeEffect := range textRangeEffects {
		textRange := text[textRangeEffect.Start:textRangeEffect.End]
		if len(textRange) != 0 {
			formattedText = formattedText + ApplyTextRangeEffectToText(textRange, textRangeEffect.TextEffect)
		}
	}
	return formattedText
}

func ApplyTextRangeEffectToText(text string, textEffect TextEffect) string {
	output := template.HTMLEscapeString(text)
	switch t := textEffect.(type) {
	case TextEffectDefault:
	case TextEffectMention:
		username := "temp"
		server := "temp"
		output = GetMentionText(username, server, output)
	case TextEffectLink:
		output = fmt.Sprintf("<a href=\"%s\">%s</a>", t.Link, output)
	case TextEffectOTP:
	case TextEffectStyles:
		for _, style := range t.Styles {
			tag := ""
			switch style {
			case StyleBold:
				tag = "b"
			case StyleItalic:
				tag = "i"
			case StyleStrikethrough:
				tag = "s"
			case StyleUnderline:
				tag = "u"
			}
			output = fmt.Sprintf("<%s>%s<%s>", tag, output, tag)
		}
	case TextEffectAnimation:
		// TODO: Be incredibly fancy and animate with css inline? lol idk
	case TextEffectConversion:
	default:
		panic(fmt.Sprintf("invalid type: %T", t))
	}
	return output
}
