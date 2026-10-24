package xlsx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
)

// ReadCell is a cell read back by ReadSheet.
type ReadCell struct {
	Ref   string // A1 reference
	Type  string // "inlineStr" for text, "" for numbers
	Style string // style id
	Value string // text or the raw number
}

// ReadSheet reads the first sheet of a file written by Writer (inline
// strings only, no shared strings): rows of cells in order. It exists for
// tests and checks of the exports, not as a general .xlsx reader.
func ReadSheet(data []byte) ([][]ReadCell, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("xlsx: %w", err)
	}
	f, err := zr.Open("xl/worksheets/sheet1.xml")
	if err != nil {
		return nil, fmt.Errorf("xlsx: %w", err)
	}
	defer f.Close()
	var ws struct {
		Rows []struct {
			Cells []struct {
				R  string `xml:"r,attr"`
				T  string `xml:"t,attr"`
				S  string `xml:"s,attr"`
				V  string `xml:"v"`
				IS struct {
					T string `xml:"t"`
				} `xml:"is"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("xlsx: %w", err)
	}
	if err := xml.Unmarshal(raw, &ws); err != nil {
		return nil, fmt.Errorf("xlsx: %w", err)
	}
	out := make([][]ReadCell, 0, len(ws.Rows))
	for _, r := range ws.Rows {
		row := make([]ReadCell, 0, len(r.Cells))
		for _, c := range r.Cells {
			v := c.V
			if c.T == "inlineStr" {
				v = c.IS.T
			}
			row = append(row, ReadCell{Ref: c.R, Type: c.T, Style: c.S, Value: v})
		}
		out = append(out, row)
	}
	return out, nil
}
