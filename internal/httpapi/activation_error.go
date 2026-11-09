package httpapi

import (
	"errors"
	"github.com/arcvpn/arcvpn/internal/app"
	"net/http"
	"os"

	"github.com/arcvpn/arcvpn/internal/engines"
	"github.com/arcvpn/arcvpn/internal/lifecycle"
	"github.com/arcvpn/arcvpn/internal/xray"
)

func activationFailure(w http.ResponseWriter, err error) {
	var mutation *app.MutationError
	if errors.As(err, &mutation) {
		handleErr(w, err)
		return
	}
	message, code := "Не удалось применить подключение. Откройте диагностику сервера; ключи в сообщении не показываются.", "apply_failed"
	var operation *xray.OperationError
	var engine *engines.OperationError
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
	case errors.As(err, &engine):
		name := map[string]string{"wireguard": "WireGuard", "hysteria2": "Hysteria2", "amneziawg": "Служба туннеля"}[engine.Kind]
		if name == "" {
			name = "VPN"
		}
		code = "engine_" + engine.Kind + "_" + engine.Stage
		switch engine.Stage {
		case "runtime":
			message = name + ": не найден или недоступен установленный движок. Проверьте его установку и права запуска."
		case "configuration":
			message = name + ": не удалось подготовить конфигурацию. Проверьте параметры и доступы устройств."
		case "validation":
			message = name + ": установленный инструмент отклонил проверку конфигурации. Служба ещё не перезапускалась."
		case "status":
			message = name + ": не удалось прочитать состояние службы. Проверьте системный обработчик."
		case "restart":
			message = name + ": служба не запустилась. Предыдущие файлы и состояние служб восстановлены."
		case "rollback":
			message = name + ": применение завершилось ошибкой, восстановление не подтверждено. Нужна диагностика сервера."
		}
	case errors.Is(err, lifecycle.ErrPaused):
		message, code = "VPN остановлен для обслуживания. Запустите его через sudo arcvpn start.", "runtime_paused"
	case errors.Is(err, os.ErrPermission):
		message, code = "Недостаточно прав для применения подключения. Проверьте права установки в диагностике сервера.", "runtime_permissions"
	case errors.Is(err, os.ErrNotExist):
		message, code = "Не найден файл движка или установки. Проверьте установку в диагностике сервера.", "runtime_missing"
	}
	stage, outcome := "apply", "unknown"
	if operation != nil {
		stage = operation.Stage
	}
	if engine != nil {
		stage = engine.Stage
	}
	switch stage {
	case "keys", "validation":
		outcome = "rejected"
	case "restart":
		outcome = "rolled_back"
	}
	respond(w, 422, map[string]string{"error": message, "code": code, "stage": stage, "outcome": outcome})
}
