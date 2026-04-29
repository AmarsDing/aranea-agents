package telemetry

import "log"

// Setup 是 OTel 接入边界，后续接入 OpenTelemetry SDK。
func Setup(serviceName string) {
	log.Printf("telemetry setup placeholder: service=%s", serviceName)
}
