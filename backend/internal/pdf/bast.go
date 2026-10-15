package pdf

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
)

// BastOrg is the letterhead and place printed on the Berita Acara.
type BastOrg struct {
	Name, Address, Place string
}

// BastParty is a signing person on the form.
type BastParty struct {
	Name, NIK, Position string
}

// BAST renders "BERITA ACARA SERAH TERIMA BARANG" with the physical-check results
// recorded for the document. t needs Employee, FromEmployee, Details.Asset.Category
// and Inspections loaded; operator is the IT Support person who made the document.
func BAST(t *models.Transaction, operator BastParty, org BastOrg) ([]byte, error) {
	b, _, err := renderBAST(t, operator, org)
	return b, err
}

var (
	navy      = [3]int{31, 56, 100}
	paleBlue  = [3]int{221, 229, 243}
	gridBlue  = [3]int{155, 185, 225}
	grayText  = [3]int{110, 110, 110}
	bLeft     = 72.0
	bWidth    = 454.0
	bTopNext  = 66.0
	bBottom   = 765.0
	bodySize  = 10.0
	tableSize = 9.5
)

type bcell struct {
	text  string
	bold  bool
	align string
	tick  bool // draw a check mark instead of text
}

func renderBAST(t *models.Transaction, operator BastParty, org BastOrg) ([]byte, []string, error) {
	p := fpdf.New("P", "pt", "A4", "")
	p.SetMargins(bLeft, 60, 69)
	p.SetAutoPageBreak(false, 0)
	p.SetCellMargin(0)
	if NoCompression {
		p.SetCompression(false)
	}
	tr := p.UnicodeTranslatorFromDescriptor("")
	p.AliasNbPages("{nb}")
	var texts []string
	font := func(style string, size float64) { p.SetFont("Helvetica", style, size) }
	color := func(c [3]int) { p.SetTextColor(c[0], c[1], c[2]) }
	black := func() { p.SetTextColor(0, 0, 0) }

	p.SetFooterFunc(func() {
		color(grayText)
		font("", 8)
		p.SetXY(bLeft, pageH-34)
		p.CellFormat(bWidth/2, 10, tr("No. Dokumen: "+t.OrderNumber), "", 0, "L", false, 0, "")
		p.CellFormat(bWidth/2, 10, fmt.Sprintf("Halaman %d/{nb}", p.PageNo()), "", 0, "R", false, 0, "")
		black()
	})
	p.AddPage()

	// line draws one string at (x, cy centre) and records it for tests.
	line := func(x, cy, w, h float64, str, style string, size float64, align string) {
		if str == "" {
			return
		}
		font(style, size)
		texts = append(texts, str)
		p.SetXY(x, cy-h/2)
		p.CellFormat(w, h, tr(str), "", 0, align, false, 0, "")
	}
	wrap := func(str, style string, size, w float64) []string {
		if str == "" {
			return nil
		}
		font(style, size)
		return p.SplitText(tr(str), w)
	}
	// paragraph prints wrapped (optionally justified) text; returns the y after it.
	paragraph := func(y float64, str, style string, size, pitch, indent float64, align string) float64 {
		font(style, size)
		texts = append(texts, str)
		p.SetXY(bLeft+indent, y)
		p.MultiCell(bWidth-indent, pitch, tr(str), "", align, false)
		return p.GetY()
	}

	// ---- letterhead ----
	name := org.Name
	if name == "" {
		name = "Departemen IT"
	}
	line(bLeft, 73, bWidth, 20, name, "B", 16, "C")
	color(grayText)
	line(bLeft, 87, bWidth, 10, org.Address, "", 7.5, "C")
	black()
	p.SetDrawColor(navy[0], navy[1], navy[2])
	p.SetLineWidth(1.4)
	p.Line(bLeft, 105.5, bLeft+bWidth, 105.5)

	// ---- title, number, opening ----
	title, verb := "BERITA ACARA SERAH TERIMA BARANG", "serah terima"
	if t.Type == models.TxIn {
		title, verb = "BERITA ACARA PENGEMBALIAN BARANG", "pengembalian"
	}
	line(bLeft, 125, bWidth, 16, title, "B", 13, "C")
	line(bLeft, 138.5, bWidth, 12, "(Aset / Inventaris Kantor)", "", 9.5, "C")
	line(bLeft, 153, bWidth, 12, "Nomor: "+t.OrderNumber, "", 9.5, "C")

	place := org.Place
	if place == "" {
		place = "................................................"
	}
	opening := fmt.Sprintf("Pada hari ini %s, tanggal %d bulan %s tahun %d, bertempat di %s, telah dilaksanakan %s barang/aset milik perusahaan antara pihak-pihak sebagai berikut:",
		days[t.CreatedAt.Weekday()], t.CreatedAt.Day(), months[t.CreatedAt.Month()-1], t.CreatedAt.Year(), place, verb)
	paragraph(169.5, opening, "", bodySize, 11, 0, "J")

	// ---- parties ----
	emp := func(e *models.Employee) BastParty {
		if e == nil {
			return BastParty{}
		}
		return BastParty{Name: e.Name, NIK: s(e.NIK), Position: s(e.Department)}
	}
	it := operator
	if it.Position == "" {
		it.Position = "IT Support / Pengelola Aset"
	}
	var first, second BastParty
	switch t.Type {
	case models.TxIn:
		first, second = emp(t.Employee), it
	case models.TxTransfer:
		first, second = emp(t.FromEmployee), emp(t.Employee)
	default:
		first, second = it, emp(t.Employee)
	}
	dots := func(v string) string {
		if v == "" {
			return "............................................................"
		}
		return v
	}
	party := func(top float64, heading, who string, pa BastParty) {
		line(bLeft, top, bWidth, 12, heading, "B", bodySize, "L")
		for i, f := range []struct{ label, value string }{
			{"Nama Lengkap", pa.Name}, {"NIK / ID Karyawan", pa.NIK}, {"Jabatan / Divisi", pa.Position},
		} {
			cy := top + 13.5 + float64(i)*14.6
			line(bLeft, cy, 146, 12, f.label, "", bodySize, "L")
			line(bLeft+146, cy, bWidth-146, 12, ": "+dots(f.value), "", bodySize, "L")
		}
		font("I", bodySize)
		line(bLeft, top+13.5+3*14.6-2, bWidth, 12, "Selanjutnya disebut sebagai "+who+".", "I", bodySize, "L")
	}
	party(217, "I. PIHAK PERTAMA (Yang Menyerahkan)", "PIHAK PERTAMA", first)
	party(291, "II. PIHAK KEDUA (Yang Menerima)", "PIHAK KEDUA", second)

	paragraph(362, "PIHAK PERTAMA menyerahkan dan PIHAK KEDUA menerima barang dengan rincian, kondisi, dan kelengkapan sebagaimana tercantum di bawah ini:", "", bodySize, 11, 0, "J")
	line(bLeft, 398.5, bWidth, 12, "A. Rincian Barang", "B", 10.5, "L")

	// ---- generic table machinery (header repeats when a table crosses a page) ----
	y := 410.0
	setBorder := func() {
		p.SetDrawColor(gridBlue[0], gridBlue[1], gridBlue[2])
		p.SetLineWidth(0.5)
	}
	drawTick := func(x, top, w, h float64) {
		p.SetDrawColor(0, 0, 0)
		p.SetLineWidth(1.3)
		cx, cy := x+w/2, top+h/2
		p.Line(cx-4.5, cy+0.5, cx-1.5, cy+4)
		p.Line(cx-1.5, cy+4, cx+5, cy-4.5)
		setBorder()
	}
	headerRow := func(cols []float64, labels []string) {
		const h = 29.0
		x := bLeft
		for i, l := range labels {
			p.SetFillColor(navy[0], navy[1], navy[2])
			p.SetDrawColor(navy[0], navy[1], navy[2])
			p.Rect(x, y, cols[i], h, "FD")
			p.SetTextColor(255, 255, 255)
			ls := wrap(l, "B", tableSize, cols[i]-6)
			top := y + (h-float64(len(ls))*11)/2
			for j, part := range ls {
				font("B", tableSize)
				texts = append(texts, part)
				p.SetXY(x+3, top+float64(j)*11)
				p.CellFormat(cols[i]-6, 11, part, "", 0, "C", false, 0, "")
			}
			x += cols[i]
		}
		black()
		setBorder()
		y += h
	}
	newPage := func() {
		p.AddPage()
		y = bTopNext
	}
	var curCols []float64
	var curLabels []string
	beginTable := func(cols []float64, labels []string) {
		curCols, curLabels = cols, labels
		if y+29+19 > bBottom {
			newPage()
		}
		headerRow(cols, labels)
	}
	dataRow := func(cells []bcell, minH float64) {
		cols := curCols
		wrapped := make([][]string, len(cells))
		maxLines := 1
		for i, c := range cells {
			style := ""
			if c.bold {
				style = "B"
			}
			wrapped[i] = wrap(c.text, style, tableSize, cols[i]-8)
			if n := len(wrapped[i]); n > maxLines {
				maxLines = n
			}
		}
		h := float64(maxLines)*11 + 8
		if h < minH {
			h = minH
		}
		if y+h > bBottom {
			newPage()
			headerRow(curCols, curLabels)
		}
		x := bLeft
		setBorder()
		for i, c := range cells {
			p.Rect(x, y, cols[i], h, "D")
			if c.tick {
				drawTick(x, y, cols[i], h)
			}
			top := y + (h-float64(len(wrapped[i]))*11)/2
			for j, part := range wrapped[i] {
				style := ""
				if c.bold {
					style = "B"
				}
				font(style, tableSize)
				texts = append(texts, c.text)
				p.SetXY(x+4, top+float64(j)*11)
				p.CellFormat(cols[i]-8, 11, part, "", 0, c.align, false, 0, "")
			}
			x += cols[i]
		}
		y += h
	}
	sectionRow := func(label string) {
		const h = 19.0
		if y+h+19 > bBottom {
			newPage()
			headerRow(curCols, curLabels)
		}
		p.SetFillColor(paleBlue[0], paleBlue[1], paleBlue[2])
		setBorder()
		p.Rect(bLeft, y, bWidth, h, "FD")
		p.SetTextColor(navy[0], navy[1], navy[2])
		line(bLeft+4, y+h/2, bWidth-8, 11, label, "B", tableSize, "L")
		black()
		y += h
	}

	// ---- A. Rincian Barang ----
	colsA := []float64{28, 96, 78, 104, 46.5, 101}
	beginTable(colsA, []string{"No", "Nama Barang", "Merk / Tipe", "Serial Number / Asset Tag", "Jumlah", "Kondisi"})
	type unit struct {
		asset *models.Asset
		insp  *models.Inspection
		prof  string
	}
	var units []unit
	inspOf := map[uint]*models.Inspection{}
	for i := range t.Inspections {
		inspOf[t.Inspections[i].AssetID] = &t.Inspections[i]
	}
	n := len(t.Details)
	for i, d := range t.Details {
		a := d.Asset
		cat, mt, sn, qty := "", "", "", 1
		if a != nil {
			if a.Category != nil {
				cat = a.Category.Name
				if a.Category.Inspection != "" && a.Category.Inspection != models.ProfileNone {
					units = append(units, unit{a, inspOf[a.ID], a.Category.Inspection})
				}
			}
			mt = strings.TrimSpace(s(a.Brand) + " " + s(a.Model))
			sn = a.Tag
			if v := s(a.SerialNumber); v != "" {
				sn = v + " / " + a.Tag
			}
			if a.Quantity > 0 {
				qty = a.Quantity
			}
		}
		cond := conditionLabel(t, d)
		if in := inspOf[d.AssetID]; in != nil && !in.Skipped {
			cond = conclusionLabel(in.Data.Conclusion)
		}
		dataRow([]bcell{{text: fmt.Sprint(i + 1), align: "C"}, {text: cat}, {text: mt}, {text: sn}, {text: fmt.Sprint(qty), align: "C"}, {text: cond}}, 19)
	}
	for i := n; i < 5; i++ { // the template shows five rows
		dataRow([]bcell{{text: fmt.Sprint(i + 1), align: "C"}, {}, {}, {}, {}, {}}, 19)
	}
	y += 28

	// ---- B + C per inspected unit ----
	colsB := []float64{26, 217, 45, 55, 111}
	colsC := []float64{33, 160, 76, 75, 109}
	for ui, u := range units {
		tpl := models.InspectionTemplates[u.prof]
		suffix := ""
		if len(units) > 1 {
			suffix = fmt.Sprintf(" - Unit %d dari %d", ui+1, len(units))
		}
		if y+60 > bBottom {
			newPage()
		}
		line(bLeft, y+6, bWidth, 12, "B. "+tpl.Title+suffix, "B", 10.5, "L")
		unitLabel := strings.TrimSpace(s(u.asset.Brand) + " " + s(u.asset.Model))
		if unitLabel == "" {
			unitLabel = "........................................"
		}
		sn := s(u.asset.SerialNumber)
		if sn != "" {
			sn += " / "
		}
		line(bLeft, y+21, bWidth, 11, "Unit diperiksa - Merk/Tipe: "+unitLabel+"  |  Serial Number / Asset Tag: "+sn+u.asset.Tag, "", 8.5, "L")
		y += 31
		beginTable(colsB, []string{"No", "Item Pemeriksaan", "Baik", "Tidak Baik", "Keterangan"})
		results := map[string]models.CheckResult{}
		var data models.InspectionData
		if u.insp != nil {
			data = u.insp.Data
			for _, c := range data.Items {
				results[c.Code] = c
			}
		}
		for _, sec := range tpl.Sections {
			sectionRow(sec.Title)
			for i, item := range sec.Items {
				r := results[item.Code]
				label, note := item.Label, r.Note
				if item.Battery {
					if data.BatteryHealth != nil {
						label += fmt.Sprintf(": %d %%", *data.BatteryHealth)
					} else {
						label += ": ............ %"
					}
				}
				dataRow([]bcell{
					{text: fmt.Sprint(i + 1), align: "C"}, {text: label},
					{tick: r.Result == models.CheckOK}, {tick: r.Result == models.CheckBad}, {text: note},
				}, 19)
			}
		}
		if u.insp != nil && u.insp.Skipped {
			y += 6
			line(bLeft, y+6, bWidth, 11, "Pemeriksaan dilewati: "+s(u.insp.SkipReason), "I", 9, "L")
			y += 14
		}
		y += 10
		box := func(on bool) string {
			if on {
				return "[x]"
			}
			return "[ ]"
		}
		concl := data.Conclusion
		if y+30 > bBottom {
			newPage()
		}
		line(bLeft, y+6, bWidth, 11, fmt.Sprintf("Kesimpulan pemeriksaan %s:  %s Layak digunakan     %s Perlu perbaikan     %s Tidak layak",
			map[string]string{models.ProfileLaptop: "laptop", models.ProfileGeneral: "barang"}[u.prof],
			box(concl == models.ConclusionFit), box(concl == models.ConclusionRepair), box(concl == models.ConclusionUnfit)), "", 9, "L")
		y += 24

		// C. Kelengkapan
		if y+60 > bBottom {
			newPage()
		}
		line(bLeft, y+6, bWidth, 12, "C. Kelengkapan / Aksesoris"+suffix, "B", 10.5, "L")
		y += 14
		beginTable(colsC, []string{"No", "Item Kelengkapan", "Ada / Tidak", "Kondisi", "Catatan"})
		acc := map[string]models.AccessoryResult{}
		for _, a := range data.Accessories {
			acc[a.Code] = a
		}
		for i, item := range tpl.Accessories {
			a := acc[item.Code]
			present := ""
			if a.Present != nil {
				present = map[bool]string{true: "Ada", false: "Tidak"}[*a.Present]
			}
			label := item.Label
			if item.Code == "acc_other" {
				label += ": ........................"
			}
			dataRow([]bcell{{text: fmt.Sprint(i + 1), align: "C"}, {text: label}, {text: present, align: "C"}, {text: a.Condition}, {text: a.Note}}, 19)
		}
		y += 14
		notes := data.Notes
		if y+30 > bBottom {
			newPage()
		}
		if notes == "" {
			line(bLeft, y+6, bWidth, 11, "Catatan kondisi tambahan: ....................................................................................................................", "", bodySize, "L")
			y += 26
		} else {
			line(bLeft, y+6, bWidth, 11, "Catatan kondisi tambahan:", "", bodySize, "L")
			y = paragraph(y+14, notes, "", bodySize, 12, 0, "L") + 12
		}
	}

	// ---- D. Ketentuan dan Pernyataan ----
	terms := []string{
		"PIHAK KEDUA menyatakan telah memeriksa dan menerima barang dalam kondisi sebagaimana tercantum pada tabel di atas, dan tidak akan mengajukan keberatan di kemudian hari atas kondisi yang sudah dicatat.",
		"Terhitung sejak ditandatanganinya berita acara ini, tanggung jawab atas penggunaan, perawatan, dan keamanan barang beralih sepenuhnya kepada PIHAK KEDUA.",
		"Barang adalah milik perusahaan dan hanya digunakan untuk kepentingan pekerjaan. Barang tidak boleh dipindahtangankan, dijual, atau dimodifikasi tanpa izin tertulis dari perusahaan.",
		"Kerusakan atau kehilangan yang terjadi akibat kelalaian PIHAK KEDUA menjadi tanggung jawab PIHAK KEDUA sesuai kebijakan perusahaan yang berlaku.",
		"Barang wajib dikembalikan dalam kondisi baik beserta seluruh kelengkapannya pada saat masa penggunaan berakhir, mutasi, atau berakhirnya hubungan kerja, melalui berita acara pengembalian terpisah.",
		"Berita acara ini dibuat dalam rangkap 2 (dua) bermaterai secukupnya, masing-masing memiliki kekuatan hukum yang sama, dan setiap pihak memegang 1 (satu) rangkap.",
	}
	if y+40 > bBottom {
		newPage()
	}
	line(bLeft, y+6, bWidth, 12, "D. Ketentuan dan Pernyataan", "B", 10.5, "L")
	y += 16
	for i, term := range terms {
		ls := wrap(term, "", bodySize, bWidth-24)
		h := float64(len(ls))*11 + 3
		if y+h > bBottom {
			newPage()
		}
		line(bLeft, y+5.5, 20, 11, fmt.Sprintf("%d.", i+1), "", bodySize, "L")
		y = paragraph(y, term, "", bodySize, 11, 24, "L") + 3
	}
	y += 14
	closing := "Demikian Berita Acara Serah Terima Barang ini dibuat dengan sebenar-benarnya, tanpa paksaan dari pihak manapun, untuk dipergunakan sebagaimana mestinya."
	if t.Type == models.TxIn {
		closing = strings.Replace(closing, "Serah Terima", "Pengembalian", 1)
	}
	if y+20+220 > bBottom {
		newPage()
	}
	y = paragraph(y, closing, "", bodySize, 11, 0, "J") + 8

	// ---- signature grid ----
	witness := BastParty{}
	if t.Type == models.TxTransfer {
		witness = operator
	}
	cellW, cellH := bWidth/2, 106.0
	sigCell := func(x, top float64, l1, l2 string, pa BastParty) {
		p.SetDrawColor(gridBlue[0], gridBlue[1], gridBlue[2])
		p.Rect(x, top, cellW, cellH, "D")
		line(x, top+13, cellW, 11, l1, "", bodySize, "C")
		line(x, top+25, cellW, 11, l2, "B", bodySize, "C")
		who := "................................................"
		if pa.Name != "" {
			who = pa.Name
		}
		line(x, top+cellH-22, cellW, 11, "("+who+")", "", bodySize, "C")
		color(grayText)
		sub := "Nama & NIK"
		if pa.NIK != "" {
			sub = "NIK: " + pa.NIK
		}
		line(x, top+cellH-10, cellW, 10, sub, "I", 8, "C")
		black()
	}
	sigCell(bLeft, y, "Yang Menyerahkan,", "PIHAK PERTAMA", first)
	sigCell(bLeft+cellW, y, "Yang Menerima,", "PIHAK KEDUA", second)
	sigCell(bLeft, y+cellH, "Menyaksikan,", "IT Support / Pengelola Aset", witness)
	sigCell(bLeft+cellW, y+cellH, "Mengetahui,", "Atasan Langsung / HRD", BastParty{})

	var buf bytes.Buffer
	if err := p.Output(&buf); err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), texts, nil
}

func conclusionLabel(c string) string {
	switch c {
	case models.ConclusionFit:
		return "Baik / layak digunakan"
	case models.ConclusionRepair:
		return "Perlu perbaikan"
	case models.ConclusionUnfit:
		return "Tidak layak"
	}
	return ""
}
