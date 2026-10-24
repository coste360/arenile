package escpos

// Control bytes of the ESC/POS command set.
const (
	LF  byte = 0x0a
	ESC byte = 0x1b
	GS  byte = 0x1d
	DLE byte = 0x10
	EOT byte = 0x04
)

// Codepage is a printer character table (api PrinterCodepage).
type Codepage string

// Supported codepages.
const (
	CP858  Codepage = "cp858"
	CP437  Codepage = "cp437"
	CP850  Codepage = "cp850"
	CP852  Codepage = "cp852"
	CP1252 Codepage = "cp1252"
)

// Align is the horizontal justification (ESC a).
type Align byte

// Alignments.
const (
	Left   Align = 0
	Center Align = 1
	Right  Align = 2
)

// CutMode is the paper cut after a document (api PrinterCutMode).
type CutMode string

// Cut modes.
const (
	CutFull    CutMode = "full"
	CutPartial CutMode = "partial"
	CutNone    CutMode = "none"
)

// Raster is a 1-bit image ready for GS v 0: WidthBytes*8 dots per row,
// Height rows, Data row-major with the most significant bit leftmost and
// 1 = black. len(Data) == WidthBytes*Height.
type Raster struct {
	WidthBytes int
	Height     int
	Data       []byte
}

// Max raster widths in dots for the supported paper widths.
const (
	MaxDots58 = 384
	MaxDots80 = 576
)
