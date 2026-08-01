package resumes

import (
	"bytes"
	"testing"
)

func TestValidPDF(t *testing.T) {
	valid := []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\n")
	if !validPDF("candidate.PDF", valid) {
		t.Fatal("expected a PDF signature, extension, and trailer to be accepted")
	}
	for name, content := range map[string][]byte{
		"wrong extension": valid,
		"missing header":  []byte("not-a-pdf%%EOF"),
		"missing trailer": []byte("%PDF-1.7 no trailer"),
		"too large":       append([]byte("%PDF-1.7"), bytes.Repeat([]byte("a"), int(MaxPDFBytes))...),
	} {
		filename := "resume.pdf"
		if name == "wrong extension" {
			filename = "resume.docx"
		}
		if validPDF(filename, content) {
			t.Fatalf("expected %s to be rejected", name)
		}
	}
}
