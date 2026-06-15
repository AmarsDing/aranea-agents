package tools

import (
	"context"
	"fmt"
	"time"

	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type datetimeInput struct{}

type datetimeOutput struct {
	Date     string `json:"date"`
	Time     string `json:"time"`
	Timezone string `json:"timezone"`
	Unix     int64  `json:"unix"`
	Weekday  string `json:"weekday"`
	ISO8601  string `json:"iso8601"`
}

func newDatetimeTool() *trpcfunction.FunctionTool[datetimeInput, datetimeOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, _ datetimeInput) (datetimeOutput, error) {
			now := time.Now()
			zone, offset := now.Zone()
			sign := "+"
			if offset < 0 {
				sign = "-"
				offset = -offset
			}
			h, m := offset/3600, (offset%3600)/60
			tzStr := fmt.Sprintf("%s (UTC%s%02d:%02d)", zone, sign, h, m)
			return datetimeOutput{
				Date:     now.Format("2006-01-02"),
				Time:     now.Format("15:04:05"),
				Timezone: tzStr,
				Unix:     now.Unix(),
				Weekday:  now.Weekday().String(),
				ISO8601:  now.Format(time.RFC3339),
			}, nil
		},
		trpcfunction.WithName("datetime"),
		trpcfunction.WithDescription("返回当前日期、时间和时区信息。当需要知道当前时间时调用此工具。"),
	)
}
