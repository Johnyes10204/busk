package processor

import (
	"testing"
	"time"
)

func TestBolivarBirthDateLecturas_27Sep46(t *testing.T) {
	layouts := defaultDateLayouts()
	lecturas := bolivarBirthDateLecturas("27-09-46", layouts)
	if len(lecturas) == 0 {
		t.Fatal("debe parsear 27-09-46")
	}
	found := false
	for _, b := range lecturas {
		if b.Year() == 1946 && b.Month() == time.September && b.Day() == 27 {
			found = true
		}
	}
	if !found {
		t.Fatalf("esperado 1946-09-27 en lecturas: %v", lecturas)
	}
}
