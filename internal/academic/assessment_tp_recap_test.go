package academic

import (
	"math/big"
	"testing"
)

func TestExactAverageKeepsPrecisionBetweenTPAndNH(t *testing.T) {
	first, err := exactAverage([]AssessmentTPRecapActivity{
		{EffectiveValue: "60.00", HasValue: true},
		{EffectiveValue: "80.00", HasValue: true},
		{EffectiveValue: "100.00", HasValue: true},
	})
	if err != nil || first.Cmp(big.NewRat(80, 1)) != 0 || first.FloatString(2) != "80.00" {
		t.Fatalf("rata-rata TP 1: value=%v err=%v", first, err)
	}
	second, err := exactAverage([]AssessmentTPRecapActivity{{EffectiveValue: "90.00", HasValue: true}})
	if err != nil || second.Cmp(big.NewRat(90, 1)) != 0 {
		t.Fatalf("rata-rata TP 2: value=%v err=%v", second, err)
	}
	nh := new(big.Rat).Add(first, second)
	nh.Quo(nh, big.NewRat(2, 1))
	if got := nh.FloatString(2); got != "85.00" {
		t.Fatalf("NH harus menghitung rata-rata TP setara, got %s", got)
	}

	// TP pertama ditampilkan 0,01, tetapi nilai tanpa pembulatan antara
	// membuat NH akhir 0,00. Membulatkan TP lebih awal akan menghasilkan 0,01.
	precise, err := exactAverage([]AssessmentTPRecapActivity{
		{EffectiveValue: "0.00", HasValue: true},
		{EffectiveValue: "0.00", HasValue: true},
		{EffectiveValue: "0.02", HasValue: true},
	})
	if err != nil || precise.FloatString(2) != "0.01" {
		t.Fatalf("format tampilan rata-rata TP: value=%v err=%v", precise, err)
	}
	preciseNH := new(big.Rat).Quo(precise, big.NewRat(2, 1))
	if got := preciseNH.FloatString(2); got != "0.00" {
		t.Fatalf("NH harus memakai nilai TP presisi sebelum pembulatan, got %s", got)
	}
}

func TestExactAverageRequiresRecordedValues(t *testing.T) {
	if _, err := exactAverage([]AssessmentTPRecapActivity{{EffectiveValue: "", HasValue: false}}); err == nil {
		t.Fatal("nilai kosong tidak boleh diubah menjadi nol")
	}
	average, err := exactAverage([]AssessmentTPRecapActivity{{EffectiveValue: "0.00", HasValue: true}})
	if err != nil || average.Cmp(big.NewRat(0, 1)) != 0 {
		t.Fatalf("nilai nol sah: value=%v err=%v", average, err)
	}
}
