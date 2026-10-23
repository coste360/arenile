package mapping

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Row generator (spec 11.2): "create N rows of M spots, spacing X, numbering
// A–Z / 1–N, starting at…". Generate is pure: the service checks the zone,
// the map bounds and the labels already on the map.

// Row label styles, numbering schemes and directions of the generator.
const (
	RowLabelsLetters = "letters"
	RowLabelsNumbers = "numbers"

	NumberingPerRow     = "per_row"
	NumberingContinuous = "continuous"

	DirectionLTR = "ltr"
	DirectionRTL = "rtl"

	DefaultLabelFormat = "{row}{number}"

	MaxGeneratedRows        = 52
	MaxGeneratedSpotsPerRow = 200
)

// GenerateParams describes a block of rows. Zero values select defaults:
// RowLabelStyle letters, FirstRowLabel "A" (or "1"), NumberStart 1,
// Numbering per_row, Direction ltr, LabelFormat "{row}{number}", Kind
// umbrella, Shape a circle of 80% of the smaller spacing, Rotation along the
// shore, SeaSide top (Italian lidos usually draw the sea at the top; the
// service always passes the side of the map).
//
// Geometry is sea side aware: row 1 (A) is the row closest to the sea and
// (OriginX, OriginY) is the centre of its first spot. Rows move away from
// the sea by SpacingY; spots of a row run parallel to the shore by SpacingX,
// left to right (or top to bottom for a sea on the left/right) unless
// Direction is rtl:
//
//	sea top:    row step (0, +SpacingY), spot step (+SpacingX, 0)
//	sea bottom: row step (0, -SpacingY), spot step (+SpacingX, 0)
//	sea left:   row step (+SpacingY, 0), spot step (0, +SpacingX)
//	sea right:  row step (-SpacingY, 0), spot step (0, +SpacingX)
type GenerateParams struct {
	ZoneID        string
	Rows          int
	SpotsPerRow   int
	OriginX       float64
	OriginY       float64
	SpacingX      float64
	SpacingY      float64
	RowLabelStyle string
	FirstRowLabel string
	NumberStart   *int
	Numbering     string
	Direction     string
	LabelFormat   string
	Kind          string
	Asset         string // map kit illustration of the generated spots; empty = default of the kind
	Shape         *store.Shape
	Rotation      *float64
	Capacity      map[string]int
	SeaSide       string
}

// Generate returns the spots of the block (new spots: empty ID, active) or a
// *configerr.ValidationError describing invalid parameters.
func Generate(p GenerateParams) ([]SpotInput, error) {
	ve := &configerr.ValidationError{}
	applyGenerateDefaults(&p)
	if p.Rows < 1 || p.Rows > MaxGeneratedRows {
		ve.Add("rows", fmt.Sprintf("Deve essere tra 1 e %d.", MaxGeneratedRows))
	}
	if p.SpotsPerRow < 1 || p.SpotsPerRow > MaxGeneratedSpotsPerRow {
		ve.Add("spots_per_row", fmt.Sprintf("Deve essere tra 1 e %d.", MaxGeneratedSpotsPerRow))
	}
	if !finite(p.SpacingX) || p.SpacingX <= 0 {
		ve.Add("spacing_x", "Deve essere maggiore di zero.")
	}
	if !finite(p.SpacingY) || p.SpacingY <= 0 {
		ve.Add("spacing_y", "Deve essere maggiore di zero.")
	}
	if !finite(p.OriginX) {
		ve.Add("origin_x", "Numero non valido.")
	}
	if !finite(p.OriginY) {
		ve.Add("origin_y", "Numero non valido.")
	}
	if p.ZoneID == "" {
		ve.Add("zone_id", "Obbligatoria.")
	}
	if !slices.Contains(SpotKinds, p.Kind) {
		ve.Add("kind", "Tipo di postazione non valido.")
	}
	validateSpotAsset(ve, "asset", p.Asset)
	if p.Numbering != NumberingPerRow && p.Numbering != NumberingContinuous {
		ve.Add("numbering", "Valore non valido (per_row, continuous).")
	}
	if p.Direction != DirectionLTR && p.Direction != DirectionRTL {
		ve.Add("direction", "Valore non valido (ltr, rtl).")
	}
	if !slices.Contains(SeaSides, p.SeaSide) {
		ve.Add("sea_side", "Valore non valido.")
	}
	if *p.NumberStart < 0 {
		ve.Add("number_start", "Deve essere un intero non negativo.")
	}
	if p.Rotation != nil && !finite(*p.Rotation) {
		ve.Add("rotation", "Numero non valido.")
	}
	if p.Shape != nil {
		if p.Shape.Type != ShapeCircle && p.Shape.Type != ShapeRect {
			ve.Add("shape.type", "Valore non valido (circle, rect).")
		}
		if !finite(p.Shape.W) || p.Shape.W <= 0 {
			ve.Add("shape.w", "Deve essere maggiore di zero.")
		}
		if !finite(p.Shape.H) || p.Shape.H <= 0 {
			ve.Add("shape.h", "Deve essere maggiore di zero.")
		}
	}
	validateQuantities(ve, "capacity", p.Capacity)
	if !strings.Contains(p.LabelFormat, "{number}") {
		ve.Add("label_format", "Deve contenere {number}.")
	} else if p.Numbering == NumberingPerRow && p.Rows > 1 && !strings.Contains(p.LabelFormat, "{row}") {
		ve.Add("label_format", "Con la numerazione per fila deve contenere {row}.")
	}
	firstRow, err := parseFirstRow(p.RowLabelStyle, p.FirstRowLabel)
	if err != nil {
		ve.Add(err.field, err.msg)
	}
	if len(ve.Fields) > 0 {
		return nil, ve
	}

	along, across := generatorAxes(p.SeaSide, p.SpacingX, p.SpacingY)
	if p.Direction == DirectionRTL {
		along = [2]float64{-along[0], -along[1]}
	}
	rotation := 0.0
	if p.Rotation != nil {
		rotation = normalizeRotation(*p.Rotation)
	} else if p.SeaSide == SeaLeft || p.SeaSide == SeaRight {
		rotation = 90
	}
	shape := store.Shape{Type: ShapeCircle, W: round2(0.8 * math.Min(p.SpacingX, p.SpacingY))}
	shape.H = shape.W
	if p.Shape != nil {
		shape = *p.Shape
	}

	out := make([]SpotInput, 0, p.Rows*p.SpotsPerRow)
	seen := make(map[string]bool, p.Rows*p.SpotsPerRow)
	number := *p.NumberStart
	for r := 0; r < p.Rows; r++ {
		rowLabel := rowLabelAt(p.RowLabelStyle, firstRow, r)
		if utf8.RuneCountInString(rowLabel) > maxRowLabelLen {
			ve.Add("first_row_label", "Etichetta di fila troppo lunga.")
			return nil, ve
		}
		if p.Numbering == NumberingPerRow {
			number = *p.NumberStart
		}
		for k := 0; k < p.SpotsPerRow; k++ {
			label := strings.NewReplacer("{row}", rowLabel, "{number}", strconv.Itoa(number)).Replace(p.LabelFormat)
			label = strings.TrimSpace(label)
			if label == "" || utf8.RuneCountInString(label) > maxLabelLen {
				ve.Add("label_format", fmt.Sprintf("Le etichette generate devono avere da 1 a %d caratteri (es. %q).", maxLabelLen, label))
				return nil, ve
			}
			if seen[label] {
				ve.Add("label_format", fmt.Sprintf("Il formato genera etichette ripetute (%s).", label))
				return nil, ve
			}
			seen[label] = true
			out = append(out, SpotInput{
				ZoneID:   p.ZoneID,
				Label:    label,
				RowLabel: rowLabel,
				Number:   number,
				Kind:     p.Kind,
				Asset:    p.Asset,
				X:        round2(p.OriginX + float64(k)*along[0] + float64(r)*across[0]),
				Y:        round2(p.OriginY + float64(k)*along[1] + float64(r)*across[1]),
				Rotation: rotation,
				Shape:    shape,
				Capacity: maps.Clone(p.Capacity),
				MaxExtra: map[string]int{},
				Active:   true,
			})
			number++
		}
	}
	return out, nil
}

func applyGenerateDefaults(p *GenerateParams) {
	if p.RowLabelStyle == "" {
		p.RowLabelStyle = RowLabelsLetters
	}
	if p.NumberStart == nil {
		one := 1
		p.NumberStart = &one
	}
	if p.Numbering == "" {
		p.Numbering = NumberingPerRow
	}
	if p.Direction == "" {
		p.Direction = DirectionLTR
	}
	if p.LabelFormat == "" {
		p.LabelFormat = DefaultLabelFormat
	}
	if p.Kind == "" {
		p.Kind = "umbrella"
	}
	if p.SeaSide == "" {
		p.SeaSide = SeaTop
	}
	if p.Capacity == nil {
		p.Capacity = map[string]int{}
	}
}

// generatorAxes returns the spot step (along the shore) and the row step
// (away from the sea).
func generatorAxes(seaSide string, sx, sy float64) (along, across [2]float64) {
	switch seaSide {
	case SeaLeft:
		return [2]float64{0, sx}, [2]float64{sy, 0}
	case SeaRight:
		return [2]float64{0, sx}, [2]float64{-sy, 0}
	case SeaBottom:
		return [2]float64{sx, 0}, [2]float64{0, -sy}
	default: // top
		return [2]float64{sx, 0}, [2]float64{0, sy}
	}
}

type paramError struct{ field, msg string }

// parseFirstRow returns the zero-based index of the first row label: for
// letters A=0 … Z=25, AA=26 (bijective base 26); for numbers the number.
func parseFirstRow(style, first string) (int, *paramError) {
	first = strings.TrimSpace(first)
	switch style {
	case RowLabelsLetters:
		if first == "" {
			return 0, nil
		}
		first = strings.ToUpper(first)
		n := 0
		for _, c := range first {
			if c < 'A' || c > 'Z' {
				return 0, &paramError{"first_row_label", "Usa solo lettere (es. A)."}
			}
			n = n*26 + int(c-'A'+1)
		}
		if len(first) > 3 {
			return 0, &paramError{"first_row_label", "Massimo 3 lettere."}
		}
		return n - 1, nil
	case RowLabelsNumbers:
		if first == "" {
			return 1, nil
		}
		n, err := strconv.Atoi(first)
		if err != nil || n < 0 || n > 9999 {
			return 0, &paramError{"first_row_label", "Usa un numero tra 0 e 9999."}
		}
		return n, nil
	default:
		return 0, &paramError{"row_label_style", "Valore non valido (letters, numbers)."}
	}
}

// rowLabelAt returns the label of row r (zero based) after the first one.
func rowLabelAt(style string, first, r int) string {
	if style == RowLabelsNumbers {
		return strconv.Itoa(first + r)
	}
	return lettersLabel(first + r)
}

// lettersLabel converts a zero-based index to A, B, …, Z, AA, AB, ….
func lettersLabel(i int) string {
	var b []byte
	for n := i + 1; n > 0; n = (n - 1) / 26 {
		b = append([]byte{byte('A' + (n-1)%26)}, b...)
	}
	return string(b)
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }
