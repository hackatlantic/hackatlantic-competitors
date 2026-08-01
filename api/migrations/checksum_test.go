package migrations

import "testing"

func TestMigrationChecksumNormalizesLineEndings(t *testing.T) {
	lf := []byte("CREATE TABLE ats.example (id integer);\nINSERT INTO ats.example VALUES (1);\n")
	crlf := []byte("CREATE TABLE ats.example (id integer);\r\nINSERT INTO ats.example VALUES (1);\r\n")

	if got, want := migrationChecksum(crlf), migrationChecksum(lf); got != want {
		t.Fatalf("CRLF checksum %s does not match LF checksum %s", got, want)
	}
}
