// Package escpos is a pure ESC/POS command encoder (ADR 0021): no I/O, no
// clock, deterministic output covered by golden byte fixtures
// (testdata/*.bin). It knows nothing about Arenile documents; layouts live
// in internal/printing/templates.
//
// Ownership: ESC/POS builders (encoder). Planned API:
//
//	e := escpos.New(escpos.Config{CharsPerLine: 48, Codepage: escpos.CP858})
//	e.Init()                        // ESC @ + ESC t <table>
//	e.Align(escpos.Center).Bold(true).Size(2, 2).Line("Lido Azzurro")
//	e.Text("…")                     // UTF-8 in, transliterated to the codepage, word-wrapped
//	e.Columns("Ombrellone A12", "€ 25,00") // two-column row, right column right-aligned
//	e.QR("ARN-7F3K", escpos.QROptions{Size: 6, ECC: escpos.ECCM}) // GS ( k, model 2
//	e.Raster(img)                   // GS v 0 from a 1-bit Raster
//	e.Feed(3).Cut(escpos.CutPartial) // GS V
//	e.KickDrawer(2)                 // ESC p
//	b := e.Bytes()
//
// Codepages: CP858 (default: € at 0xD5 and Italian accented vowels), CP437,
// CP850, CP852, CP1252. Characters missing from the codepage are
// transliterated (e.g. "€" → "EUR" on CP437, curly quotes → straight) and
// never dropped silently. The ESC t table number defaults to the Epson value
// and can be overridden per printer (codepage_table) for clones.
//
// Images: Dither converts any image.Image to a Raster (Floyd-Steinberg,
// width multiple of 8, at most 384 dots on 58 mm and 576 dots on 80 mm).
package escpos
