package provider

import "testing"

func TestFreeFormLastProjection(t *testing.T) {
	body := "한글, comma, equals = 😀\nnewline, type=2, read=0, _id=999"
	in := "Row: 0 _id=5, thread_id=1, address=010, date=1, type=1, read=0, body=" + body + "\n"
	cols := []string{"_id", "thread_id", "address", "date", "type", "read", "body"}
	rows, e := ParseProjectedRows(in, cols, "body")
	if e != nil || rows[0]["body"] != body {
		t.Fatalf("%v %#v", e, rows)
	}
}
func TestMalformedProjection(t *testing.T) {
	if _, e := ParseProjectedRows("Row: 0 _id=1, body=x\n", []string{"_id", "type", "body"}, "body"); e == nil {
		t.Fatal("expected error")
	}
	if _, e := ParseProjectedRows("garbage", []string{"_id"}, ""); e == nil {
		t.Fatal("expected error")
	}
}
