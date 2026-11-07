package httpapi

import (
	"errors"
	"net/http"
	"os"

	"github.com/arcvpn/arcvpn/internal/lifecycle"
	"github.com/arcvpn/arcvpn/internal/xray"
)

func activationFailure(w http.ResponseWriter, err error) {
	message, code := "Не удалось применить подключение. Откройте диагностику сервера; ключи в сообщении не показываются.", "apply_failed"
	var operation *xray.OperationError
	switch {
	case errors.As(err, &operation):
		code = "xray_" + operation.Stage
		switch operation.Stage {
		case "keys":
			message = "Xray не смог создать ключи. Проверьте установку и права запуска Xray в диагностике сервера."
		case "validation":
			message = "Xray отклонил конфигурацию. Проверьте параметры подключения; действующая конфигурация не заменена."
		case "restart":
			message = "Xray не запустился. Проверьте свободный порт и состояние службы; предыдущая конфигурация восстановлена."
		case "rollback":
			message = "Не удалось запустить Xray и подтвердить откат. Нужна проверка службы на сервере."
		}
	case errors.Is(err, lifecycle.ErrPaused):
		message, code = "VPN остановлен для обслуживания. Запустите его через sudo arcvpn start.", "runtime_paused"
	case errors.Is(err, os.ErrPermission):
		message, code = "Недостаточно прав для применения подключения. Проверьте права установки в диагностике сервера.", "runtime_permissions"
	case errors.Is(err, os.ErrNotExist):
		message, code = "Не найден файл движка или установки. Проверьте установку в диагностике сервера.", "runtime_missing"
	}
	respond(w, 422, map[string]string{"error": message, "code": code})
}
