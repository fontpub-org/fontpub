package cli

import (
	"bytes"
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func TestParseEmbeddedFontMetadataTTF(t *testing.T) {
	body := buildTestSFNT(t, "\x00\x01\x00\x00", "Example Sans", "Bold Italic", 700, true)
	meta, ok := parseEmbeddedFontMetadata("ExampleSans-BoldItalic.ttf", body)
	if !ok {
		t.Fatalf("expected metadata")
	}
	if meta.Family != "Example Sans" || meta.Style != "italic" || meta.Weight != 700 {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
}

func TestParseEmbeddedFontMetadataOTF(t *testing.T) {
	body := buildTestSFNT(t, "OTTO", "Example Mono", "Oblique", 400, false)
	meta, ok := parseEmbeddedFontMetadata("ExampleMono-Oblique.otf", body)
	if !ok {
		t.Fatalf("expected metadata")
	}
	if meta.Family != "Example Mono" || meta.Style != "oblique" || meta.Weight != 400 {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
}

func TestParseEmbeddedFontMetadataFallbackOnInvalid(t *testing.T) {
	if _, ok := parseEmbeddedFontMetadata("bad.ttf", []byte("not-a-font")); ok {
		t.Fatalf("expected parse failure")
	}
	if _, ok := parseEmbeddedFontMetadata("font.woff2", []byte("anything")); ok {
		t.Fatalf("woff2 should fall back")
	}
}

func buildTestSFNT(t *testing.T, signature, family, subfamily string, weight uint16, italic bool) []byte {
	t.Helper()

	nameTable := buildNameTable(t, family, subfamily)
	os2Table := make([]byte, 6)
	binary.BigEndian.PutUint16(os2Table[4:6], weight)
	headTable := make([]byte, 46)
	if italic {
		binary.BigEndian.PutUint16(headTable[44:46], 0x0002)
	}

	type table struct {
		tag  string
		body []byte
	}
	tables := []table{
		{tag: "OS/2", body: os2Table},
		{tag: "head", body: headTable},
		{tag: "name", body: nameTable},
	}

	offset := 12 + len(tables)*16
	var out bytes.Buffer
	out.WriteString(signature)
	_ = binary.Write(&out, binary.BigEndian, uint16(len(tables)))
	_ = binary.Write(&out, binary.BigEndian, uint16(0))
	_ = binary.Write(&out, binary.BigEndian, uint16(0))
	_ = binary.Write(&out, binary.BigEndian, uint16(0))

	dataBlocks := make([][]byte, 0, len(tables))
	for _, table := range tables {
		padded := append([]byte(nil), table.body...)
		for len(padded)%4 != 0 {
			padded = append(padded, 0)
		}
		out.WriteString(table.tag)
		_ = binary.Write(&out, binary.BigEndian, uint32(0))
		_ = binary.Write(&out, binary.BigEndian, uint32(offset))
		_ = binary.Write(&out, binary.BigEndian, uint32(len(table.body)))
		dataBlocks = append(dataBlocks, padded)
		offset += len(padded)
	}
	for _, block := range dataBlocks {
		out.Write(block)
	}
	return out.Bytes()
}

func buildNameTable(t *testing.T, family, subfamily string) []byte {
	t.Helper()
	familyData := utf16Bytes(family)
	subfamilyData := utf16Bytes(subfamily)

	var out bytes.Buffer
	_ = binary.Write(&out, binary.BigEndian, uint16(0))
	_ = binary.Write(&out, binary.BigEndian, uint16(2))
	_ = binary.Write(&out, binary.BigEndian, uint16(6+2*12))

	writeNameRecord := func(nameID uint16, data []byte, offset uint16) {
		_ = binary.Write(&out, binary.BigEndian, uint16(3))
		_ = binary.Write(&out, binary.BigEndian, uint16(1))
		_ = binary.Write(&out, binary.BigEndian, uint16(0x0409))
		_ = binary.Write(&out, binary.BigEndian, nameID)
		_ = binary.Write(&out, binary.BigEndian, uint16(len(data)))
		_ = binary.Write(&out, binary.BigEndian, offset)
	}
	writeNameRecord(1, familyData, 0)
	writeNameRecord(2, subfamilyData, uint16(len(familyData)))
	out.Write(familyData)
	out.Write(subfamilyData)
	return out.Bytes()
}

func utf16Bytes(text string) []byte {
	u16 := utf16.Encode([]rune(text))
	out := make([]byte, 0, len(u16)*2)
	for _, value := range u16 {
		out = append(out, byte(value>>8), byte(value))
	}
	return out
}
