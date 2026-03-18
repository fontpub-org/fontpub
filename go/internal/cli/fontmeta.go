package cli

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"
)

type fontMetadata struct {
	Family    string
	Subfamily string
	Style     string
	Weight    int
}

func parseEmbeddedFontMetadata(path string, body []byte) (fontMetadata, bool) {
	format := strings.ToLower(strings.TrimPrefix(strings.ToLower(path[len(path)-len(pathExt(path)):]), "."))
	if format != "ttf" && format != "otf" {
		return fontMetadata{}, false
	}
	meta, err := parseSFNTMetadata(body)
	if err != nil {
		return fontMetadata{}, false
	}
	return meta, true
}

func parseSFNTMetadata(body []byte) (fontMetadata, error) {
	if len(body) < 12 {
		return fontMetadata{}, fmt.Errorf("sfnt too short")
	}
	tag := string(body[:4])
	if tag != "\x00\x01\x00\x00" && tag != "OTTO" {
		return fontMetadata{}, fmt.Errorf("unsupported sfnt signature")
	}
	numTables := int(binary.BigEndian.Uint16(body[4:6]))
	if len(body) < 12+16*numTables {
		return fontMetadata{}, fmt.Errorf("invalid table directory")
	}
	tables := map[string][]byte{}
	for i := 0; i < numTables; i++ {
		entry := body[12+i*16 : 12+(i+1)*16]
		tableTag := string(entry[:4])
		offset := int(binary.BigEndian.Uint32(entry[8:12]))
		length := int(binary.BigEndian.Uint32(entry[12:16]))
		if offset < 0 || length < 0 || offset+length > len(body) {
			return fontMetadata{}, fmt.Errorf("table out of range")
		}
		tables[tableTag] = body[offset : offset+length]
	}

	family, subfamily, err := parseNameTable(tables["name"])
	if err != nil {
		return fontMetadata{}, err
	}
	weight, weightOK := parseOS2Weight(tables["OS/2"])
	style, styleOK := parseHeadAndSubfamilyStyle(tables["head"], subfamily)
	if !styleOK {
		style = "normal"
	}
	if !weightOK {
		weight = inferWeightFromText(subfamily)
	}
	return fontMetadata{
		Family:    family,
		Subfamily: subfamily,
		Style:     style,
		Weight:    weight,
	}, nil
}

func parseNameTable(table []byte) (string, string, error) {
	if len(table) < 6 {
		return "", "", fmt.Errorf("name table too short")
	}
	count := int(binary.BigEndian.Uint16(table[2:4]))
	stringOffset := int(binary.BigEndian.Uint16(table[4:6]))
	if len(table) < 6+count*12 || stringOffset > len(table) {
		return "", "", fmt.Errorf("invalid name table")
	}

	type namedValue struct {
		value  string
		score  int
		nameID uint16
	}
	var bestFamily, bestSubfamily namedValue
	for i := 0; i < count; i++ {
		record := table[6+i*12 : 6+(i+1)*12]
		platformID := binary.BigEndian.Uint16(record[0:2])
		encodingID := binary.BigEndian.Uint16(record[2:4])
		languageID := binary.BigEndian.Uint16(record[4:6])
		nameID := binary.BigEndian.Uint16(record[6:8])
		length := int(binary.BigEndian.Uint16(record[8:10]))
		offset := int(binary.BigEndian.Uint16(record[10:12]))
		if stringOffset+offset+length > len(table) {
			continue
		}
		value := decodeNameString(platformID, encodingID, table[stringOffset+offset:stringOffset+offset+length])
		if value == "" {
			continue
		}
		score := 0
		if platformID == 3 {
			score += 10
		}
		if languageID == 0x0409 {
			score += 5
		}
		switch nameID {
		case 16, 1:
			if score > bestFamily.score || (score == bestFamily.score && preferredNameID(nameID) > preferredNameID(bestFamily.nameID)) {
				bestFamily = namedValue{value: value, score: score, nameID: nameID}
			}
		case 17, 2:
			if score > bestSubfamily.score || (score == bestSubfamily.score && preferredNameID(nameID) > preferredNameID(bestSubfamily.nameID)) {
				bestSubfamily = namedValue{value: value, score: score, nameID: nameID}
			}
		}
	}
	if bestFamily.value == "" {
		return "", "", fmt.Errorf("family name not found")
	}
	return bestFamily.value, bestSubfamily.value, nil
}

func preferredNameID(nameID uint16) int {
	switch nameID {
	case 16, 17:
		return 2
	case 1, 2:
		return 1
	default:
		return 0
	}
}

func decodeNameString(platformID, encodingID uint16, body []byte) string {
	switch platformID {
	case 3:
		if len(body)%2 != 0 {
			return ""
		}
		u16 := make([]uint16, 0, len(body)/2)
		for i := 0; i < len(body); i += 2 {
			u16 = append(u16, binary.BigEndian.Uint16(body[i:i+2]))
		}
		return strings.TrimSpace(string(utf16.Decode(u16)))
	case 1:
		return strings.TrimSpace(string(body))
	default:
		if encodingID == 0 && len(body)%2 == 0 {
			u16 := make([]uint16, 0, len(body)/2)
			for i := 0; i < len(body); i += 2 {
				u16 = append(u16, binary.BigEndian.Uint16(body[i:i+2]))
			}
			return strings.TrimSpace(string(utf16.Decode(u16)))
		}
		return strings.TrimSpace(string(body))
	}
}

func parseOS2Weight(table []byte) (int, bool) {
	if len(table) < 6 {
		return 0, false
	}
	weight := int(binary.BigEndian.Uint16(table[4:6]))
	if weight < 1 || weight > 1000 {
		return 0, false
	}
	return weight, true
}

func parseHeadAndSubfamilyStyle(table []byte, subfamily string) (string, bool) {
	lower := strings.ToLower(subfamily)
	switch {
	case strings.Contains(lower, "oblique"):
		return "oblique", true
	case strings.Contains(lower, "italic"):
		return "italic", true
	}
	if len(table) >= 46 {
		macStyle := binary.BigEndian.Uint16(table[44:46])
		if macStyle&0x0002 != 0 {
			return "italic", true
		}
	}
	return "normal", true
}

func inferWeightFromText(text string) int {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "thin"):
		return 100
	case strings.Contains(lower, "extralight") || strings.Contains(lower, "ultralight"):
		return 200
	case strings.Contains(lower, "light"):
		return 300
	case strings.Contains(lower, "medium"):
		return 500
	case strings.Contains(lower, "semibold") || strings.Contains(lower, "demibold"):
		return 600
	case strings.Contains(lower, "extrabold") || strings.Contains(lower, "ultrabold"):
		return 800
	case strings.Contains(lower, "bold"):
		return 700
	case strings.Contains(lower, "black") || strings.Contains(lower, "heavy"):
		return 900
	default:
		return 400
	}
}

func pathExt(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return path[i:]
		}
		if path[i] == '/' || path[i] == '\\' {
			break
		}
	}
	return ""
}
