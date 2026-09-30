package processor

import (
	"errors"
	"testing"

	"github.com/buskseguros-design/services/api/internal/model"
	"github.com/buskseguros-design/services/api/internal/notify"
)

func TestDebeBorrarArtefactos(t *testing.T) {
	casos := []struct {
		nombre       string
		status       model.FileProcessStatus
		notifyErr    error
		delivery     notify.FileDelivery
		esperaBorrar bool
	}{
		{
			nombre:       "error con adjunto sí borra (comportamiento original)",
			status:       model.FileStatusError,
			delivery:     notify.FileDelivery{Attached: true},
			esperaBorrar: true,
		},
		{
			nombre: "error sin adjunto NO borra: el enlace del correo debe seguir vivo",
			status: model.FileStatusError,
			// Caso real de MICRO_BANCO.xlsx (30.8 MB > límite SendGrid).
			delivery:     notify.FileDelivery{Attached: false},
			esperaBorrar: false,
		},
		{
			nombre:       "fallo de envío NO borra",
			status:       model.FileStatusError,
			notifyErr:    errors.New("sendgrid status=500"),
			delivery:     notify.FileDelivery{Attached: false},
			esperaBorrar: false,
		},
		{
			nombre:       "procesado NO borra: la UI descarga el original",
			status:       model.FileStatusProcessed,
			delivery:     notify.FileDelivery{Attached: true},
			esperaBorrar: false,
		},
		{
			nombre:       "omitido NO borra",
			status:       model.FileStatusSkipped,
			delivery:     notify.FileDelivery{Attached: true},
			esperaBorrar: false,
		},
		{
			nombre:       "procesando NO borra",
			status:       model.FileStatusProcessing,
			delivery:     notify.FileDelivery{},
			esperaBorrar: false,
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := debeBorrarArtefactos(c.status, c.notifyErr, c.delivery); got != c.esperaBorrar {
				t.Fatalf("debeBorrarArtefactos()=%t, se esperaba %t", got, c.esperaBorrar)
			}
		})
	}
}

// El noop notifier (SendGrid sin configurar) no debe desaparecer nada: nadie recibió el
// archivo y nadie tiene un enlace.
func TestDebeBorrarArtefactos_NoopNotifierNoBorra(t *testing.T) {
	for _, k := range []string{"SENDGRID_API_KEY", "SENDGRID_FROM_EMAIL", "SENDGRID_ERROR_TO_EMAILS"} {
		t.Setenv(k, "")
	}

	d, err := notify.NewFileNotifierFromEnv().NotifyFileProcessing(notify.FileEmailInput{
		FileID:   "file_1",
		FileName: "x.xlsx",
		Status:   string(model.FileStatusError),
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if debeBorrarArtefactos(model.FileStatusError, nil, d) {
		t.Fatalf("con el notifier noop nada se entrega, no se debe borrar el archivo")
	}
}
