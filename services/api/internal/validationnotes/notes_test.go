package validationnotes

import "testing"

func TestSplit_InformativeVsBlocking(t *testing.T) {
	blocking, info := Split([]string{
		Incidencia("edad fuera de rango"),
		Informativo("vencimiento anterior al mes"),
		"nota blanda sin prefijo",
	})
	if len(blocking) != 1 || len(info) != 2 {
		t.Fatalf("blocking=%v info=%v", blocking, info)
	}
	if !IsBlocking(blocking[0]) || IsBlocking(info[0]) {
		t.Fatal("clasificación incorrecta")
	}
}
