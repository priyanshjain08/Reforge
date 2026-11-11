package core

import (
	"bytes"
	"fmt"
	"strings"
)

func pdfEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '(' || r == ')' || r == '\\':
			b.WriteRune('\\')
			b.WriteRune(r)
		case r < 32 || r > 126:
			b.WriteRune('?')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func wrap(line string, n int) []string {
	if len(line) <= n {
		return []string{line}
	}
	var out []string
	for len(line) > n {
		cut := strings.LastIndex(line[:n], " ")
		if cut < n/2 {
			cut = n
		}
		out = append(out, line[:cut])
		line = "    " + strings.TrimLeft(line[cut:], " ")
	}
	return append(out, line)
}

// simplePDF renders plain text lines into a minimal multi-page PDF (Helvetica, no external deps).
func simplePDF(title string, lines []string) []byte {
	var all []string
	all = append(all, title, strings.Repeat("-", 60))
	for _, l := range lines {
		all = append(all, wrap(l, 95)...)
	}
	const perPage = 52
	var pages [][]string
	for i := 0; i < len(all); i += perPage {
		j := i + perPage
		if j > len(all) {
			j = len(all)
		}
		pages = append(pages, all[i:j])
	}
	var objs []string
	kids := []string{}
	// 1 catalog, 2 pages, 3 font, then (page, content) pairs
	for i := range pages {
		kids = append(kids, fmt.Sprintf("%d 0 R", 4+i*2))
	}
	objs = append(objs, "<< /Type /Catalog /Pages 2 0 R >>")
	objs = append(objs, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pages)))
	objs = append(objs, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	for i, p := range pages {
		var cs strings.Builder
		cs.WriteString("BT /F1 9 Tf 40 800 Td 14 TL\n")
		for _, l := range p {
			cs.WriteString("(" + pdfEscape(l) + ") Tj T*\n")
		}
		cs.WriteString("ET")
		objs = append(objs, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", 5+i*2))
		objs = append(objs, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", cs.Len(), cs.String()))
	}
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", len(objs)+1, xref)
	return buf.Bytes()
}
