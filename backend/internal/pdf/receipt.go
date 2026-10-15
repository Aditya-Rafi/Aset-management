// Package pdf renders the handover/return form. The layout reproduces the
// company template "FORM SERAH TERIMA ASET KANTOR" measurement for
// measurement (A4, 2.54 cm margins, Calibri-metric font, logo top right).
package pdf

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
)

//go:embed logo.png
var logoPNG []byte

// Carlito is metric-compatible with Calibri (the template's font) and licensed under the SIL OFL (fonts/OFL.txt).
//
//go:embed fonts/Carlito-Regular.ttf
var fontRegular []byte

//go:embed fonts/Carlito-Bold.ttf
var fontBold []byte

var days = []string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}
var months = []string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}

// NoCompression keeps the content stream readable for debugging.
var NoCompression bool

// Org holds what the template fixes on the form.
type Org struct {
	Approver string // "Diketahui Oleh" — the name printed under the signature block
}

func s(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// All measurements are in points (1/72 in) and come from the template.
const (
	pageW, pageH = 595.28, 841.89
	margin       = 72.0
	textW        = pageW - 2*margin
	tableW       = 465.5 // 9310 twips: slightly wider than the text area, as in the template
	line         = 14.5  // 11 pt text at the template's 1.08 line spacing
	fontSize     = 11.0
	cellFont     = 10.0
	cellLine     = 12.4
	cellPadX     = 5.4
	cellPadY     = 4.0
	minRow       = 32.3 // 650 twips
	colonX       = 181.8
)

var assetCols = []float64{39.05, 124.65, 83.8, 86.15, 131.85} // 781/2493/1676/1723/2637 twips
var sigCols = []float64{250.55, 218.45}                       // 5011/4369 twips

type seg struct {
	text string
	bold bool
}

// conditionLabel is the "Kondisi" column: the state of the asset on this document.
func conditionLabel(t *models.Transaction, d models.TransactionDetail) string {
	if t.Type == models.TxIn {
		switch d.NewStatus {
		case models.StatusRepair:
			return "Perlu diservis"
		case models.StatusBroken:
			return "Rusak"
		}
	}
	return "Baik" // only available assets can be handed out; returns default to Tersedia
}

func assetName(a *models.Asset) []seg {
	if a == nil {
		return nil
	}
	title := strings.TrimSpace(s(a.Brand) + " " + s(a.Model))
	if title == "" && a.Category != nil {
		title = a.Category.Name
	}
	out := []seg{{a.Tag, true}}
	if title != "" {
		out = append(out, seg{title, false})
	}
	return out
}

func assetDetail(a *models.Asset, note *string) []string {
	var lines []string
	if a != nil {
		if sn := s(a.SerialNumber); sn != "" {
			lines = append(lines, "S/N: "+sn)
		}
		var spec []string
		for _, v := range []*string{a.CPU, a.RAM, a.Storage, a.OS, a.ScreenSize} {
			if x := s(v); x != "" {
				spec = append(spec, x)
			}
		}
		if len(spec) > 0 {
			lines = append(lines, strings.Join(spec, " / "))
		}
	}
	if n := s(note); n != "" {
		lines = append(lines, n)
	}
	return lines
}

// Receipt builds the PDF. t needs Employee and Details.Asset.Category loaded;
// operator is the IT staff member who created the document.
func Receipt(t *models.Transaction, operator string, org Org) ([]byte, error) {
	b, _, err := render(t, operator, org)
	return b, err
}

// render also returns every string it drew, so tests can check the content.
func render(t *models.Transaction, operator string, org Org) ([]byte, []string, error) {
	p := fpdf.New("P", "pt", "A4", "")
	p.SetMargins(margin, margin, margin)
	p.SetAutoPageBreak(false, margin)
	p.SetCellMargin(0)
	if NoCompression {
		p.SetCompression(false)
	}
	p.AddUTF8FontFromBytes("Carlito", "", fontRegular)
	p.AddUTF8FontFromBytes("Carlito", "B", fontBold)
	p.AliasNbPages("{nb}")

	var texts []string
	font := func(style string, size float64) { p.SetFont("Carlito", style, size) }

	// text draws one line vertically centred on cy.
	text := func(x, cy, w float64, str string, style string, size float64, align string) {
		if str == "" {
			return
		}
		font(style, size)
		texts = append(texts, str)
		p.SetXY(x, cy-line/2)
		p.CellFormat(w, line, str, "", 0, align, false, 0, "")
	}
	wrap := func(str string, style string, size, w float64) []string {
		if str == "" {
			return nil
		}
		font(style, size)
		var out []string
		for _, l := range p.SplitText(str, w) {
			out = append(out, l)
		}
		return out
	}

	// Logo in the header (top right, inside the top margin) and a small document footer.
	p.RegisterImageOptionsReader("logo", fpdf.ImageOptions{ImageType: "png"}, bytes.NewReader(logoPNG))
	p.SetHeaderFuncMode(func() {
		p.ImageOptions("logo", pageW-margin-57.6, margin-56.4, 57.6, 56.6, false, fpdf.ImageOptions{ImageType: "png"}, 0, "")
	}, true)
	p.SetFooterFunc(func() {
		p.SetTextColor(120, 120, 120)
		font("", 8)
		p.SetXY(margin, pageH-34)
		p.CellFormat(textW/2, 10, "No. Dokumen: "+t.OrderNumber, "", 0, "L", false, 0, "")
		p.CellFormat(textW/2, 10, fmt.Sprintf("Halaman %d/{nb}", p.PageNo()), "", 0, "R", false, 0, "")
		p.SetTextColor(0, 0, 0)
	})
	p.AddPage()

	employee, dept := "", ""
	if t.Employee != nil {
		employee, dept = t.Employee.Name, s(t.Employee.Department)
	}
	if dept == "" {
		dept = "-"
	}

	// ---- title and the three fields (positions measured from the template) ----
	title := "FORM SERAH TERIMA ASET KANTOR"
	if t.Type == models.TxIn {
		title = "FORM PENGEMBALIAN ASET KANTOR"
	}
	text(margin, 81.5, textW, title, "B", 14, "C")

	date := fmt.Sprintf("%s, %d %s %d", days[t.CreatedAt.Weekday()], t.CreatedAt.Day(), months[t.CreatedAt.Month()-1], t.CreatedAt.Year())
	for _, f := range []struct {
		cy           float64
		label, value string
	}{{132.6, "Tanggal", date}, {161, "Nama Karyawan", employee}, {188, "Jabatan/Divisi", dept}} {
		text(margin, f.cy, colonX-margin, f.label, "B", fontSize, "L")
		text(colonX, f.cy, 10, ":", "B", fontSize, "L")
		text(colonX+9, f.cy, pageW-margin-colonX-9, f.value, "", fontSize, "L")
	}
	text(margin, 222, textW, "Daftar Aset", "B", fontSize, "L")

	// ---- asset table; the header repeats on every page ----
	p.SetDrawColor(0, 0, 0)
	p.SetLineWidth(0.5)
	tableTop := 238.7
	y := tableTop

	drawHeader := func(top float64) float64 {
		x := margin
		for i, h := range []string{"No", "Nama Aset", "Jumlah", "Kondisi", "Keterangan"} {
			p.Rect(x, top, assetCols[i], minRow, "D")
			text(x, top+minRow/2, assetCols[i], h, "B", fontSize, "C")
			x += assetCols[i]
		}
		return top + minRow
	}

	type cell struct {
		lines []seg
		align string
	}
	drawRow := func(cells []cell) {
		wrapped := make([][]seg, len(cells))
		maxLines := 1
		for i, c := range cells {
			w := assetCols[i] - 2*cellPadX
			for _, l := range c.lines {
				style := ""
				if l.bold {
					style = "B"
				}
				for _, part := range wrap(l.text, style, cellFont, w) {
					wrapped[i] = append(wrapped[i], seg{part, l.bold})
				}
			}
			if n := len(wrapped[i]); n > maxLines {
				maxLines = n
			}
		}
		h := float64(maxLines)*cellLine + 2*cellPadY
		if h < minRow {
			h = minRow
		}
		if y+h > pageH-margin {
			p.AddPage()
			y = drawHeader(margin + 6) // clear of the logo
		}
		x := margin
		for i, c := range cells {
			p.Rect(x, y, assetCols[i], h, "D")
			top := y + (h-float64(len(wrapped[i]))*cellLine)/2
			for j, l := range wrapped[i] {
				style := ""
				if l.bold {
					style = "B"
				}
				font(style, cellFont)
				texts = append(texts, l.text)
				p.SetXY(x+cellPadX, top+float64(j)*cellLine)
				p.CellFormat(assetCols[i]-2*cellPadX, cellLine, l.text, "", 0, c.align, false, 0, "")
			}
			x += assetCols[i]
		}
		y += h
	}

	y = drawHeader(y)
	for i, d := range t.Details {
		qty := 1
		if d.Asset != nil && d.Asset.Quantity > 0 {
			qty = d.Asset.Quantity
		}
		var detail []seg
		for _, l := range assetDetail(d.Asset, d.Note) {
			detail = append(detail, seg{l, false})
		}
		drawRow([]cell{
			{[]seg{{fmt.Sprint(i + 1), false}}, "C"},
			{assetName(d.Asset), "L"},
			{[]seg{{fmt.Sprint(qty), false}}, "C"},
			{[]seg{{conditionLabel(t, d), false}}, "C"},
			{detail, "L"},
		})
	}
	for i := len(t.Details); i < 3; i++ { // the template always shows three rows
		drawRow([]cell{{[]seg{{fmt.Sprint(i + 1), false}}, "C"}, {}, {}, {}, {}})
	}

	// ---- statement ----
	if y+29.6+22.8+3*14.6 > pageH-margin {
		p.AddPage()
		y = margin - 29.6 + 6
	}
	cy := y + 29.6
	text(margin, cy, textW, "Keterangan", "B", fontSize, "L")
	statement := "Dengan ini, aset di atas telah diterima dalam kondisi sebagaimana tercantum. Pihak yang menerima aset bertanggung jawab atas penggunaan, pemeliharaan, dan pengembalian aset sesuai dengan ketentuan yang berlaku."
	if t.Type == models.TxIn {
		statement = "Dengan ini, aset di atas telah dikembalikan dan diterima kembali oleh IT Support dalam kondisi sebagaimana tercantum."
	}
	cy += 22.8
	last := cy
	for i, l := range wrap(statement, "", fontSize, textW) {
		last = cy + float64(i)*14.6
		text(margin, last, textW, l, "", fontSize, "L")
	}
	if n := s(t.Note); n != "" {
		for i, l := range wrap("Catatan: "+n, "", fontSize, textW) {
			last += 22.8
			if i > 0 {
				last -= 22.8 - 14.6
			}
			text(margin, last, textW, l, "", fontSize, "L")
		}
	}

	// ---- signatures: kept together on one page ----
	// The template spacing needs 237 pt below the first row. If that does not fit the
	// spacing is tightened (down to 70%) before the block moves to a new page.
	sig1 := last + 36.8
	const need = 50 + 50 + 37 + 45 + 45 + 10
	k := 1.0
	if avail := pageH - margin - sig1; avail < need {
		if avail/need >= 0.7 {
			k = avail / need
		} else {
			p.AddPage()
			sig1 = margin + 24
		}
	}
	giver, receiver := operator, employee
	switch t.Type {
	case models.TxIn:
		giver, receiver = employee, operator
	case models.TxTransfer:
		giver = ""
		if t.FromEmployee != nil {
			giver = t.FromEmployee.Name
		}
	}
	row := func(cy float64, left, right, style string) {
		text(margin+1, cy, sigCols[0], left, style, fontSize, "L")
		text(margin+sigCols[0], cy, sigCols[1], right, style, fontSize, "L")
	}
	row(sig1, "Diserahkan Oleh", "Diterima Oleh", "B")
	row(sig1+50*k, "Nama: "+giver, "Nama: "+receiver, "")
	row(sig1+100*k, "Tanda Tangan:", "Tanda Tangan:", "")
	dk := sig1 + 137*k
	text(margin, dk, textW, "Diketahui Oleh", "B", fontSize, "L")
	text(margin, dk+45*k, textW, "Nama : "+org.Approver, "", fontSize, "L")
	text(margin, dk+90*k, textW, "Tanda Tangan :", "", fontSize, "L")

	var buf bytes.Buffer
	if err := p.Output(&buf); err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), texts, nil
}
