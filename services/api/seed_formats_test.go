package main

import "testing"

// Cobertura del lote SFTP abril 2026 en tools/sftpconnect/downloads.
func TestSeedFilePrefixes_CoverageDownloads(t *testing.T) {
	cases := []struct {
		file   string
		prefix string
	}{
		{"1. 5024424900103_VIDA_VOL RM-INCLUSION ABRIL2026.xlsx", contractMapfreVida},
		{"5024424900103_VIDA_VOL RM-INCLUSION ABRIL2026.xlsx", prefixMapfreInclusionVidaAbril},
		{"2. 5024524900101_ACC MEN RM-INCLUSION ABRIL2026.xlsx", contractMapfreAccMen},
		{"5024524900101_ACC MEN RM-INCLUSION ABRIL2026.xlsx", prefixMapfreInclusionAccMenVF},
		{"3. 5024524900103_CANCER RM-INCLUSION ABRIL2026.xlsx", contractMapfreCancer},
		{"5024524900103_CANCER RM-INCLUSION ABRIL2026.xlsx", prefixMapfreInclusionCancerVF},
		{"4. Deudores_Banco_Bolivar_MICRO_BANCO_ABRIL.xlsx", prefixBolivarBancoMicro},
		{"4. Deudores_Banco_Bolivar__Pyme_BANCO_ABRIL.xlsx", prefixBolivarBancoPyme},
		{"5. Deudores_ESAL_Bolivar_Pyme_ESAL_ABRIL.xlsx", prefixBolivarEsalPyme},
		{"5. Deudores_ESAL_Bolivar_micro_ESAL_ABRIL.xlsx", prefixBolivarEsalMicro},
		{"Anulacion masiva_ABRIL 2026.xlsx", prefixMapfreAnulacionMasiva},
		{"Anulación masiva_ABRIL 2026.xlsx", prefixMapfreAnulacionMasivaTilde},
		{"Anulación masiva_MAYO 2026.xlsx", prefixMapfreAnulacionMasivaTilde},
		// Legado / otros lotes
		{"INCLUSION-VIDA-MAPFRE.xlsx", prefixMapfreInclusionVida},
		{"INCLUSION-ACCIDEMENOR-MAPFRE.xlsx", prefixMapfreInclusionAccMen},
		{"INCLUSION-CANCER-MAPFRE.xlsx", prefixMapfreInclusionCancer},
		{"STOCK_MAPFRE_Marzo_vf2_2026.xlsx", prefixMapfreStock},
		{"STOCK-FEBRE-BOLIVAR.XLS", prefixBolivarStockFebrero},
	}
	for _, tc := range cases {
		if !filenameMatchesPrefix(tc.file, tc.prefix) {
			t.Fatalf("%q debe contener prefijo %q", tc.file, tc.prefix)
		}
	}
}

func TestSeedFilePrefixes_NoCrossInsurer(t *testing.T) {
	if filenameMatchesPrefix("STOCK_MAPFRE_Marzo_vf2_2026.xlsx", prefixBolivarStock) {
		t.Fatal("STOCK_MAPFRE no debe matchear prefijo Bolívar STOCK-")
	}
	if filenameMatchesPrefix("4. Deudores_Banco_Bolivar__Pyme_BANCO_ABRIL.xlsx", prefixBolivarBancoMicro) {
		t.Fatal("Pyme BANCO no debe matchear solo MICRO_BANCO")
	}
	if filenameMatchesPrefix("5024524900101_ACC MEN RM-INCLUSION ABRIL2026.xlsx", contractMapfreCancer) {
		t.Fatal("contrato ACC MEN no debe matchear contrato CANCER")
	}
	// Las dos escrituras de "Anulacion/Anulación" son literales distintos y no deben cruzarse.
	if filenameMatchesPrefix("Anulación masiva_ABRIL 2026.xlsx", prefixMapfreAnulacionMasiva) {
		t.Fatal("archivo con tilde no debe matchear el prefijo sin tilde")
	}
	if filenameMatchesPrefix("Anulacion masiva_ABRIL 2026.xlsx", prefixMapfreAnulacionMasivaTilde) {
		t.Fatal("archivo sin tilde no debe matchear el prefijo con tilde")
	}
}

func filenameMatchesPrefix(fileName, prefix string) bool {
	return containsFold(fileName, prefix)
}

func containsFold(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) &&
		indexFold(s, sub) >= 0
}

func indexFold(s, sub string) int {
	su, subu := upperASCII(s), upperASCII(sub)
	for i := 0; i+len(subu) <= len(su); i++ {
		if su[i:i+len(subu)] == subu {
			return i
		}
	}
	return -1
}

func upperASCII(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		b[i] = c
	}
	return string(b)
}
