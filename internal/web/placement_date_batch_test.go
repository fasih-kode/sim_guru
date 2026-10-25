package web

import (
	"github.com/fasih-kode/sim_guru/internal/academic"
	"testing"
	"time"
)

func TestPlacementBatchConfirmationToken(t *testing.T) {
	a := &application{options: Options{CSRFKey: []byte("01234567890123456789012345678901")}}
	p := placementBatchToken{Actor: 9, Expires: time.Now().Add(time.Minute).Unix(), Batch: academic.PlacementStartBatch{YearID: 2, Date: "2020-07-13", Reason: "Awal tahun", IDs: []int64{1, 2}}, Digest: "snapshot"}
	token := a.signPlacementBatch(p)
	got, e := a.readPlacementBatch(token, 9)
	if e != nil || got.Digest != p.Digest || len(got.Batch.IDs) != 2 {
		t.Fatal(got, e)
	}
	if _, e = a.readPlacementBatch(token, 10); e == nil {
		t.Fatal("other actor accepted")
	}
	if _, e = a.readPlacementBatch("x"+token, 9); e == nil {
		t.Fatal("tampering accepted")
	}
	p.Expires = time.Now().Add(-time.Minute).Unix()
	if _, e = a.readPlacementBatch(a.signPlacementBatch(p), 9); e == nil {
		t.Fatal("expired preview accepted")
	}
}
